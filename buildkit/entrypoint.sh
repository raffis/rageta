#!/bin/sh
set -e

# cgroup v2: this container's own processes (this script, containerd,
# buildkitd) start out attached directly to cgroup root. cgroupv2 forbids a
# cgroup from having both directly-attached processes and controller-enabled
# child cgroups ("no internal process constraint"), so nested containers
# created later via ctr/buildkit can't get a valid cgroup there. Move
# ourselves into a leaf cgroup to free up root, then delegate controllers to
# it so nested containers can actually use them. Same fix Docker-in-Docker
# images apply (see moby's hack/dind).
if [ -f /sys/fs/cgroup/cgroup.controllers ]; then
  mkdir -p /sys/fs/cgroup/init
  xargs -rn1 < /sys/fs/cgroup/cgroup.procs > /sys/fs/cgroup/init/cgroup.procs || :
  sed -e 's/ / +/g' -e 's/^/+/' < /sys/fs/cgroup/cgroup.controllers \
    > /sys/fs/cgroup/cgroup.subtree_control || :
fi

SOCK=/run/containerd/containerd.sock

echo "[entrypoint] starting containerd..."
mkdir -p /run/containerd
rm -f "${SOCK}"
containerd --config /etc/containerd/config.toml &
CONTAINERD_PID=$!
 
echo "[entrypoint] waiting for containerd socket at ${SOCK}..."
for i in $(seq 1 50); do
  [ -S "${SOCK}" ] && break
  sleep 0.2
done
 
if [ ! -S "${SOCK}" ]; then
  echo "[entrypoint] ERROR: containerd socket never appeared" >&2
  exit 1
fi
 
echo "[entrypoint] containerd is up (pid ${CONTAINERD_PID}), socket ready at ${SOCK}"
buildkitd \
  --containerd-worker=true \
  --oci-worker=false \
  --containerd-worker-addr="${SOCK}" \
  --containerd-cni-config-path=/etc/cni/net.d/10-mynet.conflist  \
  --containerd-worker-net=cni &
BUILDKITD_PID=$!
 
# Keep the container alive, and forward signals so `docker stop` works cleanly
trap "echo '[entrypoint] shutting down'; kill -TERM ${BUILDKITD_PID} ${CONTAINERD_PID} 2>/dev/null; wait" TERM INT
 
# If extra args were passed to `docker run`, exec them as an additional
# foreground process (e.g. your service-container launcher script);
# otherwise just wait on containerd + buildkitd.
if [ "$#" -gt 0 ]; then
  echo "[entrypoint] exec'ing: $@"
  "$@" &
  CMD_PID=$!
  wait ${CMD_PID}
else
  wait ${BUILDKITD_PID}
fi
 
