FROM debian:bookworm-slim
 
ARG CONTAINERD_VERSION=1.7.24
ARG RUNC_VERSION=1.2.3
ARG CNI_PLUGINS_VERSION=1.9.1
ARG BUILDKIT_VERSION=0.31.1
ARG TARGETARCH=amd64
 
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl ca-certificates iptables xz-utils git dnsmasq \
    && rm -rf /var/lib/apt/lists/*
 
# --- containerd ---
RUN curl -fsSL https://github.com/containerd/containerd/releases/download/v${CONTAINERD_VERSION}/containerd-${CONTAINERD_VERSION}-linux-${TARGETARCH}.tar.gz \
    | tar -xz -C /usr/local
 
# --- runc (OCI runtime that containerd shells out to) ---
RUN curl -fsSL -o /usr/local/sbin/runc \
    https://github.com/opencontainers/runc/releases/download/v${RUNC_VERSION}/runc.${TARGETARCH} \
    && chmod +x /usr/local/sbin/runc
 
# --- CNI plugins (bridge, portmap, host-local, loopback, etc) ---
RUN mkdir -p /opt/cni/bin \
    && curl -fsSL https://github.com/containernetworking/plugins/releases/download/v${CNI_PLUGINS_VERSION}/cni-plugins-linux-${TARGETARCH}-v${CNI_PLUGINS_VERSION}.tgz \
    | tar -xz -C /opt/cni/bin
 
# --- buildkitd + buildctl, run in-process against the same containerd instance ---
# NOTE: installed in this SAME image/container so containerd and buildkitd share
# one filesystem/mount namespace -- this is what avoids the cross-container
# overlay/rootfs mount mismatches (see README "why one container").
RUN curl -fsSL https://github.com/moby/buildkit/releases/download/v${BUILDKIT_VERSION}/buildkit-v${BUILDKIT_VERSION}.linux-${TARGETARCH}.tar.gz \
    | tar -xz -C /usr/local
 
# --- config files ---
COPY config.toml /etc/containerd/config.toml
COPY net.conflist /etc/cni/net.d/10-mynet.conflist
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh
 
RUN mkdir -p /run/containerd /var/lib/containerd
 
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]

