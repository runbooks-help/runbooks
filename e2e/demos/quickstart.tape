# Records the landing-page "quickstart" clip: the docker path from the docs, in a
# terminal. Off-camera the stale demo container is removed, so the visible run
# starts clean; the recorded bytes are Raw WebM, transcoded by the `record:demos`
# task.
#
# `docker` is the command a reader will use; on a machine without Docker the
# task puts a podman shim on PATH first. The app image carries no content of its
# own, hence the content mount — the same form the docs' quickstart uses.
Output dist/demos/raw/quickstart.webm

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
  Type "clear"
  Enter
Show

Type "docker run -d --name runbooks -p 8090:8090 -v $PWD/content:/app/content ghcr.io/runbooks-help/runbooks:latest"
Enter
Sleep 2.5s
Type "curl -s localhost:8090/llms.txt | head -n 7"
Enter
Sleep 3.5s
