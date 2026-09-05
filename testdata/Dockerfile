# Demo fixture: try `runner-eol scan ./testdata` (no auth needed) or
# `runner-eol scan ./testdata --org YOUR_ORG` to resolve EOL dates.
FROM ghcr.io/actions/actions-runner:2.335.0
ARG RUNNER_VERSION=2.334.0
RUN curl -fsSL -o runner.tgz \
    https://github.com/actions/runner/releases/download/v${RUNNER_VERSION}/actions-runner-linux-x64-${RUNNER_VERSION}.tar.gz
