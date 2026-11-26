Test complete. The teamserver never crashed (PID 184536, 26+ min uptime, 0 panics in the log).
Report: /tmp/opencode/HAVOC_MCP_LIVE_TEST_REPORT.md
Setup: The ./havoc on disk was stale (built 00:28, before commit e4a3b3e at 00:57 that added the tool registry), so I rebuilt it with the repo's own build-project.sh --target teamserver --no-setcap. Credentials came from the profile (Neo / password1234 — the profile has no mcp operator). Agent under test: 0660f7f6.
Coverage: 273 tool calls across 8 MCP sessions — 29/29 tools and all 5 protocol methods (initialize, notifications/initialized, ping, tools/list, tools/call). Every tool with its exact arguments and result is in §3 of the report.
Files created (all teamserver-side, under data/loot/2026.11.26._01:55:18/agents/0660f7f6/): the downloaded hosts (1012 B), 3 screenshot PNGs (~140–170 KB each), and the agent's own console log. No listener left behind (port 18080 released, DB back to 3 listeners), no payload-builder leftovers, no repo changes. Test harness artifacts are all in /tmp/opencode/.
What actually ran on your PC: cmd.exe /c whoami, pwd, 3 screenshots, one 3-byte xor eax,eax; ret shellcode, one cmd.exe /c echo (failed to start), plus read-only enumeration. Nothing was written to the agent's filesystem, no process killed, no token/config changed.
Issues found (ranked)
Sev	Issue
Critical	list_sessions always returns 0 and get_session always fails — teamserver sends NameID (pkg/events/demons.go:47), operator client requires DemonID (pkg/operator/client.go:347). Same mismatch for Domain/PID/PPID vs DomainName/ProcessPID/ProcessPPID.
Critical	edit_listener panics the teamserver as documented: dispatch.go:670 asserts Info["Protocol"].(string) but the MCP handler never sends Protocol; the HTTP edit path also asserts 10 more keys, so partial configs panic too. I passed Protocol:"Http" + full config to avoid killing your server.
High	start_listener with an incomplete config also panics the teamserver (dispatch.go:369-496, unchecked type assertions). The tool validates nothing.
Medium	spawn with program:"cmd.exe" fails agent-side: ERROR_FILE_NOT_FOUND — the tool doesn't resolve the executable path like the C++ client does.
Medium	teamserver_status.profile is always "" — the profile event has Info["Demon"], the client reads Info["Name"].
Medium	wait_for_output returns next_cursor: 0 on timeout, so cursor-chaining clients jump back to the start of the ring buffer.
Low	Tasking tools return the agent heartbeat instead of the task result (timed_out=false just means "an event arrived"); screenshot payload metadata only shows up in a later wait_for_output. Listener-removal event took ~6 s to propagate.
I deliberately did not run real dll/inline_execute injections, the destructive send_command verbs, or the crash-inducing edit_listener/start_listener variants — the rationale is in §7 of the report. Say the word and I'll fix I1–I3 next (I1/I5 are a small operator-client fix; I2/I3 need the MCP layer to merge the current listener config and set Protocol).
