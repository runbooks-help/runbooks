# Records the landing-page "agent-surface" clip: the read-only surface an agent
# uses — the generated index, a raw runbook, and the body search API. A throwaway
# instance is started off-camera; the recorded bytes are Raw WebM, transcoded by
# the `record:demos` task.
Output dist/demos/raw/agent-surface.webm

Set Shell "bash"
Set Width 1152
Set Height 720
Set FontSize 14
Set Padding 20
Set MarginFill "#17161d"
Set TypingSpeed 30ms
Set Framerate 30
Set Theme {"name": "Runbooks", "background": "#17161d", "foreground": "#e9eaf1", "cursor": "#6aa88d", "selection": "#3d6756", "black": "#201f29", "red": "#e0705a", "green": "#5fa887", "yellow": "#e0a94c", "blue": "#9fb0cf", "magenta": "#aca4c4", "cyan": "#6aa88d", "white": "#e9eaf1", "brightBlack": "#9a9db2", "brightRed": "#e0705a", "brightGreen": "#6aa88d", "brightYellow": "#e0a94c", "brightBlue": "#9fb0cf", "brightMagenta": "#aca4c4", "brightCyan": "#6aa88d", "brightWhite": "#ffffff"}

Hide
  Type "clear; docker run -d --name runbooks -p 8090:8090 -v $PWD/content:/app/content ghcr.io/runbooks-help/runbooks:latest >/dev/null; sleep 2"
  Enter
  Sleep 3s
Show

Type "curl -s localhost:8090/llms.txt | head -n 6"
Enter
Sleep 2.5s
Type "curl -s localhost:8090/mts-deadlock-recovery.md | head -n 10"
Enter
Sleep 3s
Type "curl -s -G --data-urlencode 'q=replication lag' localhost:8090/api/runbooks/v1/search | jq '{count, top: [.results[0:3][].slug]}'"
Enter
Sleep 3.5s
