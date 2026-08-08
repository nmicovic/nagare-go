package setup

import (
	"fmt"
	"os"
	"path/filepath"
)

// opencodePluginTemplate is the nagare status plugin installed into OpenCode.
// The single verb is %q, the absolute path to the nagare binary.
//
// OpenCode has no command-hook mechanism like Claude Code's settings.json
// hooks, so status reporting rides on its plugin event bus. Event names are
// passed through verbatim and mapped to states inside nagare's hook handler.
//
// Only the events nagare acts on are forwarded — the bus is chatty, and every
// forwarded event costs a process spawn.
const opencodePluginTemplate = `// Installed by "nagare-go setup". Regenerated on every run — edit nagare instead.
const NAGARE = %q

const REPORTED = new Set([
  "session.created",
  "session.status",
  "session.idle",
  "session.error",
  "permission.asked",
  "permission.replied",
])

export const NagarePlugin = async ({ $, directory, worktree }) => {
  const report = async (type) => {
    const payload = JSON.stringify({
      hook_event_name: type,
      session_id: "opencode-" + (worktree ?? directory ?? ""),
      cwd: directory ?? worktree ?? "",
    })
    try {
      await $` + "`printf %%s ${payload} | ${NAGARE} hook-state`" + `.quiet()
    } catch {
      // Status reporting is best-effort; never break the session over it.
    }
  }

  return {
    event: async ({ event }) => {
      if (REPORTED.has(event.type)) await report(event.type)
    },
  }
}
`

// installOpenCodePlugin writes the nagare status plugin into OpenCode's global
// plugin directory, where it loads for every project.
func installOpenCodePlugin(home, nagareBin string) error {
	dir := filepath.Join(home, ".config", "opencode", "plugins")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, "nagare.js")
	content := fmt.Sprintf(opencodePluginTemplate, nagareBin)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return err
	}
	fmt.Printf("  Plugin: OpenCode — %s\n", path)
	return nil
}
