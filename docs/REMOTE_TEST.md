# Remote provisioning test — 2026-10-23

Tested over SSH on Ubuntu 26.04.1 LTS, Linux x86-64, using a real user systemd
session and the host's installed SteamCMD. No simulation or fake commands were
used for these checks. The existing CSM agent and CS2 server were left running;
their original process IDs were preserved.

| Check | Result |
| --- | --- |
| Fresh Steam app 730 download and validation | Passed; about 73 GB downloaded |
| Cancel installation from CLI supervision | Passed; process exited, partial files stayed unready |
| Cancel and resume through the real API/agent | Passed; heartbeat stayed online and state returned to uninstalled |
| Block local CLI mutations while an agent owns the instance | Passed |
| Generate/load a real systemd user unit | Passed after fixing WorkingDirectory quoting |
| Start and stop through the API | Passed; desired and observed states converged |
| Query live CS2 over UDP on the SSH host | Passed; expected hostname, app 730, Dust2 and then Mirage |
| Repeat Start without restarting the process | Passed; process ID unchanged |
| Reject map changes while running | Passed with HTTP 409 |
| Stop, change map, restart | Passed; game query and API both reported Mirage |
| Stop agent without stopping its running game | Passed |
| Reject an update of a running game | Passed; game process unchanged |
| Update a stopped instance and start through standalone CLI | Passed |
| Final stop and cleanup | Passed; game PID zero, test UDP listener absent, test agent stopped |

Two implementation changes were required:

- `WorkingDirectory=` must contain an unquoted path. It does not use the same
  word-unquoting rules as `ExecStart=`. Installation now also checks systemd's
  `LoadState=loaded` before writing its installed marker.
- CS2 resets game cvars while loading a map. Managed instances now put settings
  in `server.cfg`, which the engine executes after the reset. The startup
  `clutchbase.cfg` loads settings, supplies any configured GSLT, and starts the
  map. Map commands stay out of `server.cfg` to avoid recursive map loads.

Regression tests were added for both changes. The full Go race suite, real
PostgreSQL integration tests, `go vet`, frontend production build, and local
binary builds passed afterward.

UDP queries from the development machine to the game port timed out even after
the server answered queries locally. No firewall or routing rules were changed.
An actual player connection, password authentication, and public internet
operation with a GSLT were not tested. The instance used an anonymous Steam
server login, for which the engine reported LAN-only player connections.

A2S reported its password-protection flag as false. This is not a reliable
authentication check in CS2: [Valve issue #3993](https://github.com/ValveSoftware/csgo-osx-linux/issues/3993)
describes the same reporting defect even with `sv_password` set. The test does
not claim that a query flag proves password authentication works.

The approximately 69 GiB installation is retained, stopped, under
`~/.local/share/clutchbase-test-20261023`. Its instance is `cb-smoke-20261023`,
using game port 27235 and reserved client/TV ports 27236–27237. The generated
game service is `clutchbase-cb-smoke-20261023.service`, disabled at boot. Tools
are in that root's `bin/` directory. Private instance credentials remain on the
host; temporary API-agent credentials and the SSH forwarding session were
removed after testing. No SSH password is stored in this repository.
