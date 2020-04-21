FROM ubuntu:18.04
RUN apt-get update && \
    apt-get install -y build-essential curl emacs git nano
RUN curl -L -o /usr/local/bin/bazel https://github.com/bazelbuild/bazel/releases/download/3.0.0/bazel-3.0.0-linux-x86_64 && \
    chmod +x /usr/local/bin/bazel && \
    bazel version
RUN git clone https://github.com/bazelbuild/rules_go rules_go
