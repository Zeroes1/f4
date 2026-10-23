# Folder bookmarks

Ten numbered slots (`Ctrl+Shift+<digit>` saves the active panel's location, `RCtrl+<digit>` or `Ctrl+Alt+<digit>` jumps to it; the bookmarks dialog lists them). The file is far2l's `bookmarks.ini`, so it can be shared with far2l unchanged.

## What a slot remembers

* **Path**: the location of the active panel, as an address the panel can be brought back to. For the local disk that is the directory, for an archive the path inside it; for the Docker, Kubernetes and MongoDB panels it is `docker:///<container>/…`, `k8s:///<namespace>/<pod>/<container>/…`, `mongo:///<database>/<collection>` (each plugin registers a URI provider, see [DOCKER.md](DOCKER.md), [KUBERNETES.md](KUBERNETES.md), [MONGODB.md](MONGODB.md)), and folder history and saved sessions use the same addresses. A NetFox session gives the address of its saved connection and the folder on the server, `net://<connection name>/<folder>`, whatever the connection's type, so the bookmark reconnects with the password, key, proxy and codepage of the connection; a session opened from an `sftp://`, `scp://` or `smb://` address gives that address without its password, with the folder ([NETFOX.md](NETFOX.md)). In the NetFox connection manager a bookmark taken with the cursor on a connection is that connection, `net://<name>`, so a connection is bookmarked without being opened and the bookmark opens it in its start folder; with the cursor elsewhere in the list it is the manager itself, `net://`. A VFS names such an address through `vfs.BookmarkPathProvider`; one that does not is bookmarked by its path. Only bookmarks and links ask: saved sessions and folder history keep their own rules, so a start of f4 never opens a connection on its own.
* **Plugin / PluginData** (f4#1669): when a panel plugin covers the folder (Git status, the process list, ...), the slot also records the plugin's panel provider as `Plugin=f4-panel:<ID>` and the panel's own state (filter, selected item) in `PluginData`. Jumping to the slot goes to the folder and opens that plugin over it; if the plugin is not available (disabled, not installed here) only the folder is opened and a message says so. far2l's own `Plugin` values are kept as they are and never mistaken for f4's.

## The dialog

Each row shows the slot digit and the path, and `[ID]` after it for a plugin bookmark. `Ins` or `Ctrl+N` saves the current location, `Del` clears a slot, `Shift+Up`/`Shift+Down` move it, `F4` edits the path: for a plugin bookmark the edit box names the plugin in its title, and changing the path moves the bookmark while it keeps opening the same plugin (`Del` and saving again drops the plugin).

## Drive-menu links

The named links of the drive menu (Alt+F1/Alt+F2; `drive-bookmarks.ini`) store a path only, in the same form as above, so a link made on a Docker, Kubernetes or MongoDB panel, or on a NetFox connection (`net://<name>/<folder>`, offered as the path of a new link made with `Ins` while the panel is on the server, or with the cursor on the connection in the manager), brings that panel back. In the drive menu, Ctrl+Up and Ctrl+Down move the link under the cursor one place up or down, and the order is saved to the file. The same keys move a tool row (a plugin such as AI, Android or Network) among the tool rows; that order is kept one name per line in `settings/drive-tools-order.txt` of the profile, and applies while the "sort plugins by hotkey" option is off.

### Global hot key (f4#2171)

A link has two keys, both set in its dialog (`Ins` or `F4` in the drive menu). The **hot key** is one character that picks the link while the menu is open. The **global hot key** is any key with Ctrl or Alt, or a function key, that opens the link in the active panel without the menu. The two are independent: `Alt+F1`, then `D` can go to Downloads while `Ctrl+Alt+D` goes there at once. There is no separate table of link keys: the global hot key is stored with the link, as `GlobalHotkey=` in `drive-bookmarks.ini`, in the spelling of the hotkey table (`CtrlAltD`, `RCtrlY`, `CtrlShiftF11`). The drive menu shows it at the right edge of the link's row.

The global hot key field takes the key pressed in it; nothing is typed there. `Del` or `Backspace` clears it, and `Tab`, `Enter` and `Esc` work as in any other field. A key without Ctrl or Alt that is not a function key is refused, because the panels and the command line need it.

When the key already does something in the panels, the dialog lists what and offers **Take** or **Choose another**. Take frees the key on Save, never before:

* a command bound to the key in `Shell` or `Common` gets `None` in `Shell` in `hotkeys.ini`, so a `Common` binding keeps working in the editor and the viewer;
* another link with the key loses its global hot key;
* a macro on the key, quick search (Alt with a letter) and switching workspaces (Alt with a digit) keep their keys, but in the panels the link comes first;
* so do the built-in keys, the ones the panels and vtui handle themselves (the rows marked `•` in the Hotkey Configurator: `Ctrl+N`, `Ctrl+Tab`, the keys of the focused quick view), and the plugins' own hot keys. A built-in key that works only in some state says which (`when QuickViewFocused`).

Some keys are refused outright, because they are handled before the links are asked: the numbered bookmark keys (`RCtrl` or `Ctrl+Alt` with a digit or `` ` ``, and the same with Shift), macro recording (`Ctrl+.`), and a key that `keymap.ini` or f4's Mac layout turns into another before the key router sees it. A key the link already has is checked again on Save, since a command may have been bound to it in the meantime.

In the panels the key router asks the links after the command palette, macro recording and the bookmark keys, and before macros, plugin keys, the hotkey table and the panels' own keys. A link written with `Ctrl` answers either Ctrl key. One written with `RCtrl` answers the right Ctrl only; it takes the right Ctrl from a command, a macro or a built-in key on plain `Ctrl`, which answer either Ctrl, and leaves them the left one. With the panels hidden, the key goes to the terminal.

Before f4#2171 the dialog had a "Quick call with RCtrl/Ctrl+Alt" check box. A link saved with it (`QuickCall=true`) and a Latin letter as its hot key now loads with that letter on RCtrl as its global hot key. The next save writes `GlobalHotkey=` and drops `QuickCall`. The `Ctrl+Alt` variant is not carried over; set it again in the dialog if needed.

The main Settings window edits the global hot key as a chord field with Capture, and the Links page of the drive menu's F9 window as a field the key is pressed in. Neither can ask whether to take a key, so Apply refuses a global hot key that is new to the file and that something else already has, and points to the link dialog instead. A key the file already gives the link is not asked about again.
