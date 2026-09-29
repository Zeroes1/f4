# MongoDB panel (`plugins/mongofs`)

[f4#1663](https://github.com/unxed/f4/issues/1663), MongoDB part: a server as a read-only drive.

Open it from the drive menu (Alt+F1) as **MongoDB**. Databases and collections are folders; the documents of a collection are `<id>.json` files (relaxed extended JSON, indented) that F3 views and F5 copies out.

## How

* **No driver.** The official Go driver is a big dependency for what a browser needs. The plugin speaks the wire protocol itself: OP_MSG (MongoDB 3.6+) with a small BSON codec, and `listDatabases`, `listCollections`, `find`, `getMore` and `killCursors`.
* **Connection:** the `MONGODB_URI` environment variable, default `mongodb://127.0.0.1:27017`. `mongodb://[user:pass@]host[:port][/db][?authSource=x&tls=true]`; with a seed list the first host is used. Authentication is SCRAM-SHA-256 (the default since MongoDB 4.0); SCRAM-SHA-1, X.509, Kerberos, `mongodb+srv://` and replica-set discovery are not supported yet.
* **Names.** An ObjectId `_id` is the 24-digit hex name; a string id is `s_<escaped>`, an integer id `i_<n>`, anything else `j_<escaped json>` (listed and openable while the listing is fresh). Listings ask only for `_id`; a document is fetched when opened.
* **Limits.** A collection lists its first 1000 documents and then says the list is partial. Read-only. Nothing connects until the panel is opened.

## Not yet

Editing, queries/filters, `mongodb+srv://`, other auth mechanisms, the lite build (full build only, like the Docker and Kubernetes panels).
