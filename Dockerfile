FROM golang:1.23.4 AS builder
WORKDIR /app

ARG VERSION

COPY go.mod /app/go.mod
COPY go.sum /app/go.sum
RUN go mod download

COPY ./ ./

RUN make build

FROM debian:bookworm

WORKDIR /app
ENV DEBIAN_FRONTEND=noninteractive

RUN --mount=type=cache,target=/var/lib/apt \
    --mount=type=cache,target=/var/cache/apt \
    apt-get update  && \
    apt-get install --no-install-recommends -y curl unzip ca-certificates zip tzdata wget gnupg2 bzip2 apt-transport-https locales locales-all lsb-release git python3-crcmod python3-openssl

RUN locale-gen en_US.UTF-8
RUN update-locale LANG=en_US.UTF-8

ENV GCLOUD_PATH=/opt/google-cloud-sdk
ENV PATH $GCLOUD_PATH/bin:$PATH
ENV CLOUDSDK_PYTHON=/usr/bin/python3
# Download and install cloud sdk. Review the components I install, you may not need them.
RUN GCLOUDCLI_URL="https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-cli-linux-x86_64.tar.gz" && \
if [ "${TARGETARCH}" = "arm64" ]; then \
  GCLOUDCLI_URL="https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-cli-linux-arm.tar.gz"; \
fi && \
    curl $GCLOUDCLI_URL -o gcloud.tar.gz && \
    tar xzf gcloud.tar.gz -C /opt && \
    rm gcloud.tar.gz && \
    rm -rf $GCLOUD_PATH/platform/bundledpythonunix && \
    gcloud config set core/disable_usage_reporting true && \
    gcloud config set component_manager/disable_update_check true && \
    gcloud config set metrics/environment github_docker_image && \
    gcloud components remove -q bq && \
    gcloud components install -q beta kubectl-oidc gke-gcloud-auth-plugin && \
    rm -rf $(find $GCLOUD_PATH/ -regex ".*/__pycache__") && \
    rm -rf $GCLOUD_PATH/.install/.backup && \
    rm -rf $GCLOUD_PATH/bin/anthoscli && \
    gcloud --version


FROM    debian:bookworm

# Install all locales

WORKDIR /app
ENV DEBIAN_FRONTEND=noninteractive

RUN --mount=type=cache,target=/var/lib/apt \
    --mount=type=cache,target=/var/cache/apt \
    apt-get update  && \
    apt-get install --no-install-recommends -y curl unzip ca-certificates zip tzdata wget gnupg2 bzip2 apt-transport-https locales locales-all lsb-release git python3-crcmod python3-openssl

RUN locale-gen en_US.UTF-8
RUN update-locale LANG=en_US.UTF-8

# stern, jq, yq
RUN curl -sLS https://get.arkade.dev | sh && \
  arkade get kubectl jq yq sops --path /usr/bin && \
  chmod +x /usr/bin/kubectl /usr/bin/jq /usr/bin/yq /usr/bin/sops

ENV GCLOUD_PATH=/opt/google-cloud-sdk
ENV PATH $GCLOUD_PATH/bin:$PATH
ENV CLOUDSDK_PYTHON=/usr/bin/python3

COPY --from=gcloud-installer /opt/google-cloud-sdk /opt/google-cloud-sdk

# Azure CLI
RUN mkdir -p /etc/apt/keyrings && \
  curl -sLS https://packages.microsoft.com/keys/microsoft.asc | \
    gpg --dearmor | tee /etc/apt/keyrings/microsoft.gpg > /dev/null && \
  chmod go+r /etc/apt/keyrings/microsoft.gpg &&  \
  echo "deb [arch=`dpkg --print-architecture` signed-by=/etc/apt/keyrings/microsoft.gpg] https://packages.microsoft.com/repos/azure-cli/ $(lsb_release -cs) main" | tee /etc/apt/sources.list.d/azure-cli.list && \
  cat /etc/apt/sources.list.d/azure-cli.list && \
  apt-get update && \
  apt-get install -y azure-cli && \
  apt-get clean && \
  rm -rf $(find /opt/az -regex ".*/__pycache__") && \
  az version

# AWS CLI
RUN AWSCLI_URL="https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" && \
  if [ "${TARGETARCH}" = "arm64" ]; then \
    AWSCLI_URL="https://awscli.amazonaws.com/awscli-exe-linux-aarch64.zip"; \
  fi && \
  curl "${AWSCLI_URL}" -o "awscliv2.zip" && \
  unzip -q awscliv2.zip && ./aws/install -i /opt/aws -b /usr/bin/ && \
  rm awscliv2.zip && \
  rm -rf ./aws && \
  aws --version

COPY --from=builder /app/batch-runner /app/batch-runner

ENTRYPOINT ["/app/batch-runner"]
