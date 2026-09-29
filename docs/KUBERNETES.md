# Kubernetes panel (`plugins/k8sfs`)

[f4#1663](https://github.com/unxed/f4/issues/1663), Kubernetes part: a cluster as a read-only drive.

Open it from the drive menu (Alt+F1) as **Kubernetes**. Namespaces are the top-level folders, then pods, then a pod's containers; inside a container is its file system. F3 and F5 work as on any panel; nothing can be changed yet.

## How

* **kubectl-free.** The API server is spoken to directly with `net/http`: `GET /api/v1/namespaces` and `/api/v1/namespaces/{ns}/pods` for the tree. No client-go (a huge dependency tree) and no `kubectl` binary.
* **Files through exec.** The API has no file endpoint, so, like `kubectl exec` and `kubectl cp`, the plugin runs commands in the container over the exec WebSocket (`v4.channel.k8s.io`, via `golang.org/x/net/websocket`, which f4 already depends on): `ls -1ApL` for names and folder flags, `stat -c` for sizes, times and modes, `cat` for content (copied to a temporary file so F3/F5 get random access). The container needs `ls`, `stat` and `cat`; busybox has them all. Images without them (distroless) can be browsed down to the container but not into it.
* **Credentials from a kubeconfig** (`KUBECONFIG`, first file, or `~/.kube/config`): the current context's server, CA (file or inline), bearer token (or `tokenFile`), client certificate and key. Users whose credentials come from a helper program (`exec:` plugins such as `gke-gcloud-auth-plugin`, `aws eks get-token`, or the old `auth-provider`) are refused with a message saying so; nothing is sent without credentials.
* Nothing is read or connected until the panel is opened.

## Not yet

Writing (copy in, mkdir, delete, rename), credential helper programs, in-cluster configuration, switching context from the panel, ephemeral/init containers, and the lite build (full build only, like the Docker panel).
