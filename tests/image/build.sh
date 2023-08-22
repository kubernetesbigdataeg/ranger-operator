podman build . --tag docker.io/kubernetesbigdataeg/ranger:2.4.0-1
podman login docker.io -u kubernetesbigdataeg
podman push docker.io/kubernetesbigdataeg/ranger:2.4.0-1
