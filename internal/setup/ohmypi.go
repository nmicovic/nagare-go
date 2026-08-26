package setup

import (
	"fmt"
	"os"
	"path/filepath"
)

// ohMyPiExtensionTemplate is the nagare status extension installed into OhMyPi.
// OhMyPi has a native MCP client, so messaging tools are registered through its
// mcp.json rather than duplicated here.
const ohMyPiExtensionTemplate = `// Installed by "nagare-go setup". Regenerated on every run — edit nagare instead.
import type { ExtensionAPI, ExtensionContext } from "@oh-my-pi/pi-coding-agent";

const NAGARE = %q;

async function report(pi: ExtensionAPI, ctx: ExtensionContext, event: string): Promise<void> {
  const payload = JSON.stringify({
    hook_event_name: event,
    session_id: ctx.sessionManager.getSessionId(),
    cwd: ctx.cwd,
  });
  try {
    await pi.exec("sh", ["-c", 'printf %%s "$1" | "$0" hook-state', NAGARE, payload], {
      timeout: 5000,
    });
  } catch {
    // Status reporting is best-effort; never break the session over it.
  }
}

export default function (pi: ExtensionAPI) {
  const statusEvents = [
    "session_start",
    "before_agent_start",
    "agent_start",
    "turn_start",
    "auto_compaction_start",
    "auto_retry_start",
    "tool_approval_requested",
    "tool_approval_resolved",
    "agent_end",
    "session_shutdown",
  ] as const;

  for (const event of statusEvents) {
    pi.on(event, async (_e: unknown, ctx: ExtensionContext) => {
      await report(pi, ctx, event);
    });
  }
}
`

// installOhMyPiExtension writes nagare's status extension into OhMyPi's global
// extension directory, where it is auto-discovered for every project.
func installOhMyPiExtension(home, nagareBin string) error {
	dir := filepath.Join(home, ".omp", "agent", "extensions")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, "nagare.ts")
	content := fmt.Sprintf(ohMyPiExtensionTemplate, nagareBin)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return err
	}
	fmt.Printf("  Extension: OhMyPi — %s\n", path)
	return nil
}
