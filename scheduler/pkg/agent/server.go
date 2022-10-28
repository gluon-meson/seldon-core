package agent

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	seldontls "github.com/seldonio/seldon-core-v2/components/tls/pkg/tls"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"

	"github.com/seldonio/seldon-core/scheduler/pkg/coordinator"

	pb "github.com/seldonio/seldon-core/scheduler/apis/mlops/agent"
	pbs "github.com/seldonio/seldon-core/scheduler/apis/mlops/scheduler"
	"github.com/seldonio/seldon-core/scheduler/pkg/scheduler"
	"github.com/seldonio/seldon-core/scheduler/pkg/store"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	grpcMaxConcurrentStreams          = 1_000_000
	pendingSyncsQueueSize         int = 10
	modelEventHandlerName             = "agent.server.models"
	modelScalingCoolingOffSeconds     = 300
)

type ServerKey struct {
	serverName string
	replicaIdx uint32
}

type Server struct {
	mutext sync.RWMutex
	pb.UnimplementedAgentServiceServer
	logger           log.FieldLogger
	agents           map[ServerKey]*AgentSubscriber
	store            store.ModelStore
	scheduler        scheduler.Scheduler
	certificateStore *seldontls.CertificateStore
}

type SchedulerAgent interface {
	modelSync(modelName string) error
}

type AgentSubscriber struct {
	finished chan<- bool
	mutex    sync.Mutex // grpc streams are not thread safe for sendMsg https://github.com/grpc/grpc-go/issues/2355
	stream   pb.AgentService_SubscribeServer
}

func NewAgentServer(
	logger log.FieldLogger,
	store store.ModelStore,
	scheduler scheduler.Scheduler,
	hub *coordinator.EventHub,
) *Server {
	s := &Server{
		logger:    logger.WithField("source", "AgentServer"),
		agents:    make(map[ServerKey]*AgentSubscriber),
		store:     store,
		scheduler: scheduler,
	}

	hub.RegisterModelEventHandler(
		modelEventHandlerName,
		pendingSyncsQueueSize,
		s.logger,
		s.handleSyncs,
	)

	return s
}

func (s *Server) handleSyncs(event coordinator.ModelEventMsg) {
	logger := s.logger.WithField("func", "handleSyncs")
	logger.Infof("Received sync for model %s", event.String())

	// TODO - Should this spawn a goroutine?
	// Surely we're risking reordering of events, e.g. load/unload -> unload/load?
	go s.Sync(event.ModelName)
}

func (s *Server) startServer(port uint, secure bool) error {
	logger := s.logger.WithField("func", "startServer")
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	opts := []grpc.ServerOption{}
	if secure {
		opts = append(opts, grpc.Creds(s.certificateStore.CreateServerTransportCredentials()))
	}
	opts = append(opts, grpc.MaxConcurrentStreams(grpcMaxConcurrentStreams))
	opts = append(opts, grpc.UnaryInterceptor(otelgrpc.UnaryServerInterceptor()))
	grpcServer := grpc.NewServer(opts...)
	pb.RegisterAgentServiceServer(grpcServer, s)
	s.logger.Printf("Agent server running on %d mtls:%v", port, secure)
	go func() {
		err := grpcServer.Serve(lis)
		if err != nil {
			logger.WithError(err).Fatalf("Agent server failed on port %d mtls:%v", port, secure)
		} else {
			logger.Infof("Agent serving stopped on port %d mtls:%v", port, secure)
		}
	}()
	return nil
}

func (s *Server) StartGrpcServer(allowPlainTxt bool, agentPort uint, agentTlsPort uint) error {
	logger := s.logger.WithField("func", "StartGrpcServer")
	var err error
	protocol := seldontls.GetSecurityProtocolFromEnv(seldontls.EnvSecurityPrefixControlPlane)
	if protocol == seldontls.SecurityProtocolSSL {
		s.certificateStore, err = seldontls.NewCertificateStore(seldontls.Prefix(seldontls.EnvSecurityPrefixControlPlaneServer),
			seldontls.ValidationPrefix(seldontls.EnvSecurityPrefixControlPlaneClient))
		if err != nil {
			return err
		}
	}
	if !allowPlainTxt && s.certificateStore == nil {
		return fmt.Errorf("One of plain txt or mTLS needs to be defined. But have plain text [%v] and no TLS", allowPlainTxt)
	}
	if allowPlainTxt {
		err := s.startServer(agentPort, false)
		if err != nil {
			return err
		}
	} else {
		logger.Info("Not starting scheduler plain text server")
	}
	if s.certificateStore != nil {
		err := s.startServer(agentTlsPort, true)
		if err != nil {
			return err
		}
	} else {
		logger.Info("Not starting scheduler mTLS server")
	}
	return nil
}

func (s *Server) Sync(modelName string) {
	logger := s.logger.WithField("func", "Sync")
	s.mutext.RLock()
	defer s.mutext.RUnlock()
	s.store.LockModel(modelName)
	defer s.store.UnlockModel(modelName)

	model, err := s.store.GetModel(modelName)
	if err != nil {
		logger.WithError(err).Error("Sync failed")
		return
	}
	if model == nil {
		logger.Errorf("Model %s not found", modelName)
		return
	}

	// Handle any load requests for latest version - we don't want to load models from older versions
	latestModel := model.GetLatest()
	if latestModel != nil {
		for _, replicaIdx := range latestModel.GetReplicaForState(store.LoadRequested) {
			logger.Infof("Sending load model request for %s", modelName)

			as, ok := s.agents[ServerKey{serverName: latestModel.Server(), replicaIdx: uint32(replicaIdx)}]

			if !ok {
				logger.Errorf("Failed to find server replica for %s:%d", latestModel.Server(), replicaIdx)
				continue
			}

			as.mutex.Lock()
			err = as.stream.Send(&pb.ModelOperationMessage{
				Operation:    pb.ModelOperationMessage_LOAD_MODEL,
				ModelVersion: &pb.ModelVersion{Model: latestModel.GetModel(), Version: latestModel.GetVersion()},
			})
			as.mutex.Unlock()
			if err != nil {
				logger.WithError(err).Errorf("stream message send failed for model %s and replicaidx %d", modelName, replicaIdx)
				continue
			}
			err := s.store.UpdateModelState(latestModel.Key(), latestModel.GetVersion(), latestModel.Server(), replicaIdx, nil, store.LoadRequested, store.Loading, "")
			if err != nil {
				logger.WithError(err).Errorf("Sync set model state failed for model %s replicaidx %d", modelName, replicaIdx)
				continue
			}
		}
	}

	// Loop through all versions and unload any requested - any version of a model might have an unload request
	for _, modelVersion := range model.Versions {
		for _, replicaIdx := range modelVersion.GetReplicaForState(store.UnloadRequested) {
			s.logger.Infof("Sending unload model request for %s:%d", modelName, modelVersion.GetVersion())
			as, ok := s.agents[ServerKey{serverName: modelVersion.Server(), replicaIdx: uint32(replicaIdx)}]
			if !ok {
				logger.Errorf("Failed to find server replica for %s:%d", modelVersion.Server(), replicaIdx)
				continue
			}
			as.mutex.Lock()
			err = as.stream.Send(&pb.ModelOperationMessage{
				Operation:    pb.ModelOperationMessage_UNLOAD_MODEL,
				ModelVersion: &pb.ModelVersion{Model: modelVersion.GetModel(), Version: modelVersion.GetVersion()},
			})
			as.mutex.Unlock()
			if err != nil {
				logger.WithError(err).Errorf("stream message send failed for model %s and replicaidx %d", modelName, replicaIdx)
				continue
			}
			err := s.store.UpdateModelState(modelVersion.Key(), modelVersion.GetVersion(), modelVersion.Server(), replicaIdx, nil, store.UnloadRequested, store.Unloading, "")
			if err != nil {
				logger.WithError(err).Errorf("Sync set model state failed for model %s replicaidx %d", modelName, replicaIdx)
				continue
			}
		}
	}
}

func (s *Server) AgentEvent(ctx context.Context, message *pb.ModelEventMessage) (*pb.ModelEventResponse, error) {
	logger := s.logger.WithField("func", "AgentEvent")
	var desiredState store.ModelReplicaState
	var expectedState store.ModelReplicaState
	switch message.Event {
	case pb.ModelEventMessage_LOADED:
		expectedState = store.Loading
		desiredState = store.Loaded
	case pb.ModelEventMessage_UNLOADED:
		expectedState = store.Unloading
		desiredState = store.Unloaded
	case pb.ModelEventMessage_LOAD_FAILED,
		pb.ModelEventMessage_LOAD_FAIL_MEMORY:
		expectedState = store.Loading
		desiredState = store.LoadFailed
	case pb.ModelEventMessage_UNLOAD_FAILED:
		expectedState = store.Unloading
		desiredState = store.UnloadFailed
	default:
		desiredState = store.ModelReplicaStateUnknown
	}
	logger.Infof("Updating state for model %s to %s", message.ModelName, desiredState.String())
	s.store.LockModel(message.ModelName)
	defer s.store.UnlockModel(message.ModelName)
	err := s.store.UpdateModelState(message.ModelName, message.GetModelVersion(), message.ServerName, int(message.ReplicaIdx), &message.AvailableMemoryBytes, expectedState, desiredState, message.GetMessage())
	if err != nil {
		logger.WithError(err).Infof("Failed Updating state for model %s", message.ModelName)
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &pb.ModelEventResponse{}, nil
}

func (s *Server) ModelScalingTrigger(stream pb.AgentService_ModelScalingTriggerServer) error {
	for {
		message, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&pb.ModelScalingTriggerResponse{})
		}
		if err != nil {
			return err
		}
		logger := s.logger.WithField("func", "ModelScalingTrigger")
		logger.Infof("Received Event from server %s:%d for model %s:%d",
			message.GetServerName(), message.GetReplicaIdx(), message.GetModelName(), message.GetModelVersion())

		// so far we do not care about oder of scaling events. the first one should win
		go func() {
			if err := s.applyModelScaling(message); err != nil {
				logger.WithError(err).Debugf("Could not scale model %s", message.GetModelName())
			}
		}()
	}
}

func (s *Server) Subscribe(request *pb.AgentSubscribeRequest, stream pb.AgentService_SubscribeServer) error {
	logger := s.logger.WithField("func", "Subscribe")
	logger.Infof("Received subscribe request from %s:%d", request.ServerName, request.ReplicaIdx)

	fin := make(chan bool)

	s.mutext.Lock()
	s.agents[ServerKey{serverName: request.ServerName, replicaIdx: request.ReplicaIdx}] = &AgentSubscriber{
		finished: fin,
		stream:   stream,
	}
	s.mutext.Unlock()

	err := s.syncMessage(request, stream)
	if err != nil {
		return err
	}

	ctx := stream.Context()
	// Keep this scope alive because once this scope exits - the stream is closed
	for {
		select {
		case <-fin:
			logger.Infof("Closing stream for replica: %s:%d", request.ServerName, request.ReplicaIdx)
			return nil
		case <-ctx.Done():
			logger.Infof("Client replica %s:%d has disconnected", request.ServerName, request.ReplicaIdx)
			s.mutext.Lock()
			delete(s.agents, ServerKey{serverName: request.ServerName, replicaIdx: request.ReplicaIdx})
			s.mutext.Unlock()
			modelsChanged, err := s.store.RemoveServerReplica(request.ServerName, int(request.ReplicaIdx))
			if err != nil {
				logger.WithError(err).Errorf("Failed to remove replica and redeploy models for %s:%d", request.ServerName, request.ReplicaIdx)
			}
			s.logger.Debugf("Models changed by disconnect %v", modelsChanged)
			for _, modelName := range modelsChanged {
				err = s.scheduler.Schedule(modelName)
				if err != nil {
					logger.Debugf("Failed to reschedule model %s when server %s replica %d disconnected", modelName, request.ServerName, request.ReplicaIdx)
				}
			}
			return nil
		}
	}
}

func (s *Server) syncMessage(request *pb.AgentSubscribeRequest, stream pb.AgentService_SubscribeServer) error {
	s.mutext.Lock()
	defer s.mutext.Unlock()

	s.logger.Debugf("Add Server Replica %+v with config %+v", request, request.ReplicaConfig)
	err := s.store.AddServerReplica(request)
	if err != nil {
		return err
	}
	_, err = s.scheduler.ScheduleFailedModels()
	if err != nil {
		return err
	}
	return nil
}

func (s *Server) applyModelScaling(message *pb.ModelScalingTriggerMessage) error {

	modelName := message.ModelName
	model, err := s.store.GetModel(modelName)
	if err != nil {
		return err
	}
	if model == nil {
		return fmt.Errorf("Model %s not found", modelName)
	}

	// TODO: consider the case when scaling down a model that failed to scale up, hence not available
	lastAvailableModelVersion := model.GetLastAvailableModel()
	if lastAvailableModelVersion == nil {
		return fmt.Errorf("Stable model version %s not found", modelName)
	}

	modelProto, err := createScalingPseudoRequest(message, lastAvailableModelVersion)
	if err != nil {
		return err
	}

	return s.updateAndSchedule(modelProto)
}

func (s *Server) updateAndSchedule(modelProtos *pbs.Model) error {
	modelName := modelProtos.GetMeta().GetName()
	if err := s.store.UpdateModel(&pbs.LoadModelRequest{
		Model: modelProtos,
	}); err != nil {
		return err
	}

	return s.scheduler.Schedule(modelName)
}

func createScalingPseudoRequest(message *pb.ModelScalingTriggerMessage, lastAvailableModelVersion *store.ModelVersion) (*pbs.Model, error) {
	//TODO: update model state

	modelName := message.ModelName

	if lastAvailableModelVersion.GetVersion() != message.GetModelVersion() {
		return nil, fmt.Errorf(
			"Model version %s not matching (expected: %d - actual: %d)",
			modelName, lastAvailableModelVersion.GetVersion(), message.GetModelVersion())
	}

	modelProtos := lastAvailableModelVersion.GetModel() // this is a clone of the protos
	numReplicas := len(lastAvailableModelVersion.GetAssignment())

	if !isModelStable(lastAvailableModelVersion) {
		return nil, fmt.Errorf("Model %s has changed status recently, skip scaling", modelName)
	}

	if desiredNumReplicas, err := calculateDesiredNumReplicas(modelProtos, message.Trigger, numReplicas); err != nil {
		return nil, err
	} else {
		modelProtos.DeploymentSpec.Replicas = uint32(desiredNumReplicas)
	}
	return modelProtos, nil
}

func isModelStable(modelVersion *store.ModelVersion) bool {
	return modelVersion.ModelState().Timestamp.Before(time.Now().Add(-modelScalingCoolingOffSeconds * time.Second))
}

func calculateDesiredNumReplicas(model *pbs.Model, trigger pb.ModelScalingTriggerMessage_Trigger, numReplicas int) (int, error) {

	if trigger == pb.ModelScalingTriggerMessage_SCALE_UP {
		if err := checkModelScalingWithinRange(model, numReplicas+1); err != nil {
			return 0, err
		} else {
			return numReplicas + 1, nil
		}
	} else if trigger == pb.ModelScalingTriggerMessage_SCALE_DOWN {
		if err := checkModelScalingWithinRange(model, numReplicas-1); err != nil {
			return 0, err
		} else {
			return numReplicas - 1, nil
		}
	}
	return 0, fmt.Errorf("event not supported")
}

// we autoscale if at least min or max replicas is set and that we are within the range
// if a user therefore sets only the number of replicas then autoscaling will not be activated
// which is hidden in this logic unfortunately as we reject the scaling up / down event.
// a side effect is that we do not go below 1 replica of a model
func checkModelScalingWithinRange(model *pbs.Model, targetNumReplicas int) error {
	minReplicas := model.DeploymentSpec.GetMinReplicas()
	maxReplicas := model.DeploymentSpec.GetMaxReplicas()

	if (minReplicas == 0) && (maxReplicas == 0) {
		// no autoscaling
		return fmt.Errorf("No autoscaling for model %s", model.GetMeta().GetName())
	}

	if targetNumReplicas < int(minReplicas) || (targetNumReplicas < 1) {
		return fmt.Errorf("Violating min replicas %d / %d for model %s", minReplicas, targetNumReplicas, model.GetMeta().GetName())
	}

	if targetNumReplicas > int(maxReplicas) && (maxReplicas > 0) {
		return fmt.Errorf("Violating max replicas %d / %d for model %s", maxReplicas, targetNumReplicas, model.GetMeta().GetName())
	}

	return nil
}