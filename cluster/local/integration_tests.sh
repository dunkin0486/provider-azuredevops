#!/usr/bin/env bash
set -e

# setting up colors
BLU='\033[0;34m'
YLW='\033[0;33m'
GRN='\033[0;32m'
RED='\033[0;31m'
NOC='\033[0m' # No Color
echo_info(){
    printf "\n${BLU}%s${NOC}" "$1"
}
echo_step(){
    printf "\n${BLU}>>>>>>> %s${NOC}\n" "$1"
}
echo_sub_step(){
    printf "\n${BLU}>>> %s${NOC}\n" "$1"
}

echo_step_completed(){
    printf "${GRN} [✔]${NOC}"
}

echo_success(){
    printf "\n${GRN}%s${NOC}\n" "$1"
}
echo_warn(){
    printf "\n${YLW}%s${NOC}" "$1"
}
echo_error(){
    printf "\n${RED}%s${NOC}" "$1"
    exit 1
}


# The name of your provider. Many provider Makefiles override this value.
PACKAGE_NAME="provider-azuredevops"


# ------------------------------
projectdir="$( cd "$( dirname "${BASH_SOURCE[0]}")"/../.. && pwd )"

# get the build environment variables from the special build.vars target in the main makefile
eval $(make --no-print-directory -C ${projectdir} build.vars)

# ------------------------------

SAFEHOSTARCH="${SAFEHOSTARCH:-amd64}"
# BUILD_IMAGE is also the runtime image embedded in the xpkg by
# `xpkg.mk`'s `--embed-runtime-image` (see `make build.all`), so it's the
# only image we need to load into kind -- there is no separate "-controller"
# image with modern (embedded-runtime) provider packages.
BUILD_IMAGE="${BUILD_REGISTRY}/${PROJECT_NAME}-${SAFEHOSTARCH}"

K8S_CLUSTER="${K8S_CLUSTER:-${BUILD_REGISTRY}-inttests}"

CROSSPLANE_NAMESPACE="crossplane-system"

# cleanup on exit
if [ "$skipcleanup" != true ]; then
  function cleanup {
    echo_step "Cleaning up..."
    export KUBECONFIG=
    "${KIND}" delete cluster --name="${K8S_CLUSTER}"
  }

  trap cleanup EXIT
fi

# setup package cache
echo_step "setting up local package cache"
CACHE_PATH="${projectdir}/.work/inttest-package-cache"
mkdir -p "${CACHE_PATH}"
echo "created cache dir at ${CACHE_PATH}"

# Extract straight from the .xpkg file that `make build.all` already wrote
# to disk, rather than from the docker daemon. Extracting from the daemon
# via crank/go-containerregistry is unreliable on Docker Desktop when the
# containerd image store is enabled (produces a "failed to open package
# stream file: EOF" error), so --from-xpkg is used instead -- it reads the
# OCI layout directly and works regardless of the daemon's image store.
XPKG_DIR="${projectdir}/_output/xpkg/linux_${SAFEHOSTARCH}"
XPKG_FILE="$(ls -t "${XPKG_DIR}"/"${PROJECT_NAME}"-*.xpkg 2>/dev/null | head -1)"
[ -n "${XPKG_FILE}" ] || echo_error "no .xpkg file found in ${XPKG_DIR}; run 'make build.all' first"
echo_info "using package ${XPKG_FILE}"

# Modern Crossplane (crossplane-runtime v2's CachedClient) always resolves a
# tag-based package reference to a digest via a registry HEAD request before
# it will even consult its local package cache -- packagePullPolicy: Never
# only skips the *pull*, not this digest lookup. It skips the lookup (and any
# network access) only when given an actual digest reference
# (repo@sha256:...) up front. We exploit that for fully offline testing: mint
# an arbitrary sha256 "digest" from the .xpkg file's own contents, reference
# the package by that digest, and store the extracted package contents under
# the exact cache filename Crossplane derives for that (source, digest) pair
# -- FriendlyID()/ToDNSLabel() from crossplane-runtime's pkg/xpkg/name.go,
# reimplemented here in Python since it's not exposed as a CLI/API.
PACKAGE_SOURCE="local.xpkg/${PROJECT_NAME}"
PACKAGE_DIGEST="sha256:$( (sha256sum "${XPKG_FILE}" 2>/dev/null || shasum -a 256 "${XPKG_FILE}") | awk '{print $1}')"
PACKAGE_REF="${PACKAGE_SOURCE}@${PACKAGE_DIGEST}"
CACHE_KEY="$(python3 - "${PACKAGE_SOURCE}" "${PACKAGE_DIGEST}" <<'PYEOF'
import sys


def truncate(s, n):
    return s[:n]


def to_dns_label(s):
    out = []
    n = len(s)
    for i, b in enumerate(s):
        if ("a" <= b <= "z") or ("0" <= b <= "9"):
            out.append(b)
        if b in ".:/-" and i != 0 and i != 62 and i != n - 1:
            out.append("-")
        if i == 62:
            break
    return "".join(out).strip("-")


name, digest = sys.argv[1], sys.argv[2]
print(to_dns_label("-".join([truncate(name, 50), truncate(digest, 12)])))
PYEOF
)"
echo_info "package ref=${PACKAGE_REF} cache-key=${CACHE_KEY}"
"${CROSSPLANE_CLI}" xpkg extract --from-xpkg "${XPKG_FILE}" -o "${CACHE_PATH}/${CACHE_KEY}.gz"
chmod 644 "${CACHE_PATH}/${CACHE_KEY}.gz"


# create kind cluster with extra mounts
KIND_NODE_IMAGE="kindest/node:${KIND_NODE_IMAGE_TAG}"
echo_step "creating k8s cluster using kind ${KIND_VERSION} and node image ${KIND_NODE_IMAGE}"
KIND_CONFIG="$( cat <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraMounts:
  - hostPath: "${CACHE_PATH}/"
    containerPath: /cache
EOF
)"
echo "${KIND_CONFIG}" | "${KIND}" create cluster --name="${K8S_CLUSTER}" --wait=5m --image="${KIND_NODE_IMAGE}" --config=-

# load the (already built) provider image directly into kind -- it's the
# same image embedded as the runtime image inside the xpkg above, so no
# separate tagging/renaming is needed.
"${KIND}" load docker-image "${BUILD_IMAGE}" --name="${K8S_CLUSTER}"

echo_step "create crossplane-system namespace"
"${KUBECTL}" create ns crossplane-system

echo_step "create persistent volume and claim for mounting package-cache"
PV_YAML="$( cat <<EOF
apiVersion: v1
kind: PersistentVolume
metadata:
  name: package-cache
  labels:
    type: local
spec:
  storageClassName: manual
  capacity:
    storage: 5Mi
  accessModes:
    - ReadWriteOnce
  hostPath:
    path: "/cache"
EOF
)"
echo "${PV_YAML}" | "${KUBECTL}" create -f -

PVC_YAML="$( cat <<EOF
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: package-cache
  namespace: crossplane-system
spec:
  accessModes:
    - ReadWriteOnce
  volumeName: package-cache
  storageClassName: manual
  resources:
    requests:
      storage: 1Mi
EOF
)"
echo "${PVC_YAML}" | "${KUBECTL}" create -f -

# install crossplane from stable channel
# NOTE: the build submodule no longer vendors a Helm binary (no $(HELM3)
# variable), so this relies on `helm` being available on your PATH.
echo_step "installing crossplane from stable channel"
command -v helm >/dev/null 2>&1 || echo_error "helm is required on PATH to run acceptance tests locally"
helm repo add crossplane-stable https://charts.crossplane.io/stable/ --force-update
helm repo update crossplane-stable
chart_version="$(helm search repo crossplane-stable/crossplane | awk 'FNR == 2 {print $2}')"
echo_info "using crossplane version ${chart_version}"
echo
# we replace empty dir with our PVC so that the /cache dir in the kind node
# container is exposed to the crossplane pod
helm install crossplane --namespace crossplane-system crossplane-stable/crossplane --version ${chart_version} --wait --set packageCache.pvc=package-cache

# ----------- integration tests
echo_step "--- INTEGRATION TESTS ---"

# install package
echo_step "installing ${PROJECT_NAME} into \"${CROSSPLANE_NAMESPACE}\" namespace"

# The extracted package we cached above has no OCI manifest for Crossplane to
# read an embedded-runtime-image annotation from, so the package manager
# defaults to using the package's own (fake, digest-pinned) reference as the
# controller/runtime image too -- which was never loaded into kind and can't
# be pulled offline. A DeploymentRuntimeConfig lets us override just the
# runtime container image to the real, already kind-loaded ${BUILD_IMAGE}.
RUNTIME_CONFIG_YAML="$( cat <<EOF
apiVersion: pkg.crossplane.io/v1beta1
kind: DeploymentRuntimeConfig
metadata:
  name: "${PACKAGE_NAME}-runtime"
spec:
  deploymentTemplate:
    spec:
      selector: {}
      template:
        spec:
          containers:
            - name: package-runtime
              image: "${BUILD_IMAGE}:latest"
              imagePullPolicy: Never
EOF
)"

INSTALL_YAML="$( cat <<EOF
apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: "${PACKAGE_NAME}"
spec:
  package: "${PACKAGE_REF}"
  packagePullPolicy: Never
  runtimeConfigRef:
    name: "${PACKAGE_NAME}-runtime"
EOF
)"

echo "${RUNTIME_CONFIG_YAML}" | "${KUBECTL}" apply -f -
echo "${INSTALL_YAML}" | "${KUBECTL}" apply -f -

# printing the cache dir contents can be useful for troubleshooting failures
echo_step "check kind node cache dir contents"
docker exec "${K8S_CLUSTER}-control-plane" ls -la /cache

echo_step "waiting for provider to be installed"

kubectl wait "provider.pkg.crossplane.io/${PACKAGE_NAME}" --for=condition=healthy --timeout=180s

echo_step "uninstalling ${PROJECT_NAME}"

echo "${INSTALL_YAML}" | "${KUBECTL}" delete -f -
echo "${RUNTIME_CONFIG_YAML}" | "${KUBECTL}" delete -f -

# check pods deleted
timeout=60
current=0
step=3
while [[ "$(kubectl get providerrevision.pkg.crossplane.io -o name | wc -l | tr -d '[:space:]')" != "0" ]]; do
  echo "waiting for provider to be deleted for another $step seconds"
  current=$((current + step))
  if ! [[ $timeout -gt $current ]]; then
    echo_error "timeout of ${timeout}s has been reached"
  fi
  sleep $step;
done

echo_success "Integration tests succeeded!"
