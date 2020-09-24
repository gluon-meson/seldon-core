from setuptools import setup, find_packages

tests_require = ["pytest", "pytest-tornasync", "mypy", "requests-mock"]
setup(
    name="adserver",
    version="0.1.0",
    author_email="cc@seldon.io",
    license="https://github.com/seldonio/seldon-core/LICENSE",
    url="https://github.com/seldonio/seldon-core/components/alibi-detect-server",
    description="Alibi Detect Server for use with cloud events",
    python_requires=">3.4",
    packages=find_packages(),
    install_requires=[
        "seldon-core>=1.2.3"
        "alibi-detect==0.4.2",
        "kfserving==0.4.0",
        "argparse >= 1.4.0",
        "numpy >= 1.18.5",
        "cloudevents==0.3.0",
        "fbprophet==0.6",
        "holidays==0.9.11",
    ],
    tests_require=tests_require,
    extras_require={"test": tests_require},
)