# Docker panel (`plugins/dockerfs`)

Part 1 of [f4#1663](https://github.com/unxed/f4/issues/1663): Docker containers as a read-only drive.

Open it from the drive menu (Alt+F1) as **Docker**. The top level lists all containers (running or not) as folders; inside one is that container's file system. F3, F5 and Enter work as on any panel; nothing can be changed yet.

## Why this shape

* **Built-in Go plugin, standard library only.** The Docker Engine speaks plain HTTP on a unix socket (or `tcp://`). Linking the Docker SDK would add a large dependency tree for four endpoints, and shelling out to the `docker` CLI would need the CLI to be installed. Neither is needed: `net/http` with a unix-socket dialer is enough, so the plugin adds no dependency, needs no CGO and no external tool. It follows `plugins/sqlite` and `plugins/ios` (in-process plugin, `vfs.VFS` implementation, registered with the host).
* **One VFS, POSIX paths.** `/` lists containers, `/<container>` is a container's root, the rest is the path inside it. Containers are looked up by name (the short id for an unnamed one).
* **Reading via the archive endpoint** (`/containers/{id}/archive`, what `docker cp` uses): `HEAD` gives a path's stat in a header, `GET` gives a tar. It works on stopped containers too and needs nothing inside the image (no shell, no `ls`). Symlinks (`/bin -> usr/bin`) are followed by the plugin, so they can be entered like folders.
* **Files are copied out to a temporary file** on `Open`, because the endpoint only streams and F3/F5 need random access.

## Connection

`DOCKER_HOST` as the CLI reads it: `unix:///path/to.sock` or `tcp://host:port` (plain HTTP). Unset: `/var/run/docker.sock`, then the rootless `$XDG_RUNTIME_DIR/docker.sock`. Not supported yet: Windows named pipes, `ssh://`, TLS. Nothing connects until the panel is opened.

## Limits of this part

* Read-only (copy out, view). Copying into a container, mkdir, delete, rename come later.
* The archive endpoint has no "one level only" mode: listing a folder streams the tar of everything under it. Listing `/` of a big image is slow, and after 400000 entries the listing stops and says it is partial.
* The full build only. The lite build (see `internal/plughost/plugins_lite.go`) does not include it yet.

## Next parts

Kubernetes (pods and containers through the API server, same panel shape) and MongoDB (databases and collections as folders and files), each as its own plugin in its own part.
