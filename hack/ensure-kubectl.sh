#!/usr/bin/env bash

# Copyright 2021 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

# shellcheck source=./hack/utils.sh
source "$(dirname "$0")/utils.sh"

GOPATH_BIN="$(go env GOPATH)/bin/"
MINIMUM_KUBECTL_VERSION=v1.16.7

# Expected sha256 for the linux/amd64 kubectl binary at MINIMUM_KUBECTL_VERSION.
# Update this from https://dl.k8s.io/release/<version>/bin/linux/amd64/kubectl.sha256
# whenever MINIMUM_KUBECTL_VERSION changes.
# shellcheck disable=SC2034 # read via indirect expansion below
KUBECTL_SHA256_v1_16_7_linux_amd64="c31ca51b526489cd929be71fc1dc9c3cc24b6df5641b3505b467bac51862047d"

# Ensure the kubectl tool exists and is a viable version, or installs it
verify_kubectl_version() {

  # If kubectl is not available on the path, get it
  if ! [ -x "$(command -v kubectl)" ]; then
    if [[ "${OSTYPE}" == "linux-gnu" ]]; then
      if ! [ -d "${GOPATH_BIN}" ]; then
        mkdir -p "${GOPATH_BIN}"
      fi
      echo 'kubectl not found, installing'
      # ${MINIMUM_KUBECTL_VERSION//./_} replaces every "." in the version (e.g.
      # "v1.16.7") with "_", since "." is not valid in a bash variable name but
      # the pinned constant above is.
      KUBECTL_SHA256_VAR="KUBECTL_SHA256_${MINIMUM_KUBECTL_VERSION//./_}_linux_amd64"
      KUBECTL_SHA256="${!KUBECTL_SHA256_VAR:?no known sha256 for kubectl ${MINIMUM_KUBECTL_VERSION} on linux/amd64, add it to $0}"
      download_and_verify "https://dl.k8s.io/release/${MINIMUM_KUBECTL_VERSION}/bin/linux/amd64/kubectl" "${KUBECTL_SHA256}" "${GOPATH_BIN}/kubectl"
      chmod +x "${GOPATH_BIN}/kubectl"
    else
      echo "Missing required binary in path: kubectl"
      return 2
    fi
  fi

  local kubectl_version
  IFS=" " read -ra kubectl_version <<< "$(kubectl version --client)"
  if [[ "${MINIMUM_KUBECTL_VERSION}" != $(echo -e "${MINIMUM_KUBECTL_VERSION}\n${kubectl_version[2]}" | sort -s -t. -k 1,1 -k 2,2n -k 3,3n | head -n1) ]]; then
    cat <<EOF
Detected kubectl version: ${kubectl_version[2]}.
Requires ${MINIMUM_KUBECTL_VERSION} or greater.
Please install ${MINIMUM_KUBECTL_VERSION} or later.
EOF
    return 2
  fi
}

verify_kubectl_version
