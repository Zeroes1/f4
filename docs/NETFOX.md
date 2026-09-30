# NetFox connection types

The NetFox plugin (the connection manager, and the `sftp://`, `scp://` and `smb://` addresses) offers these connection types. Connections are stored in the manager and reopened from bookmarks, folder history and saved sessions through their URI.

| Type | Address | Notes |
| --- | --- | --- |
| FTP | connection manager | |
| SFTP | `sftp://[user@]host[:port]/path` | key, agent or password |
| SCP | `scp://[user@]host[:port]/path` | opens the SFTP backend (below) |
| SMB | `smb://[domain;]user[:password]@host[:port]/share/path` | Windows and Samba shares |
| FISH+ | connection manager (type `fish+`, `fish` is a synonym) | see [FISH+.md](FISH+.md) |

## SCP (f4#187)

`scp://` and the connection type **SCP** are the SFTP backend under the name people expect from `scp`: an SSH server that speaks SCP today also speaks SFTP, and SFTP gives real file listing, random access and rename, which the SCP protocol has no wire format for. A host with only the old SCP subsystem and no SFTP is not supported.

## SMB (f4#188)

`smb://[domain;]user[:password]@host[:port]/share/path`: the port is 445 unless given, the `DOMAIN;user` form of `smbclient` is understood, and without a user the logon is anonymous. Without a share (`smb://host/`) the panel lists the shares of the server. On systems other than Windows the UNC forms `\\host`, `\\host\share\dir` (and `//host/share` when there is no such local path) typed on the command line or in the path box open the same way. The panel reads, writes, creates folders, renames and deletes, and copies to and from it use the ordinary file operations. Shares themselves are made and removed on the server, not from the panel, and one rename cannot cross two shares (copy and delete instead). It is a pure Go client (`github.com/cloudsoda/go-smb2`), so no `mount` and no external tool is involved. The SMB type is in the full build only; the lite build leaves it out.

Not done yet: a password store or keyring for SMB connections, and Kerberos.
