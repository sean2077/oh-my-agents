# Porting oma Skills to a New Harness

This guide is for adapting oma's skills to another agent harness while preserving oma's core constraints: small resident surface, agent-neutral skill bodies, explicit installation, and zero host-config mutation.

## Principles

1. **Name actions, not tools.** Skill bodies should say what the agent must do: read a file, run a command, publish a relay artifact, record a verifier result. Harness-specific tool names belong in a mapping document or installer metadata, not in the shared skill text.
2. **Never edit user files.** A port must not reach into personal or global config files such as shell startup files, editor settings, trusted-folder lists, or host-owned hook config. Ship an installable artifact, document manual wiring when needed, and stop if the harness cannot carry the required context without mutating user-owned state.
3. **Keep startup context manual unless the harness owns it.** oma does not depend on hidden startup injection. If a harness offers a native extension surface, use it only for files the extension owns. If it cannot do that, document the limitation instead of adding hidden mutation.
4. **Preserve the shared skill body.** Do not rewrite `assets/skills/*/SKILL.md` just to satisfy one harness. Add a harness mapping or a narrow docs page.

## Shape A: Native Skill Surface

Use this when the harness can install skill-like assets directly.

- Package the existing `assets/skills/<name>/` content.
- Map oma's generic actions to the harness's actual tool names in a harness-specific reference.
- Keep the skill description and body unchanged unless the improvement is valid for every harness.
- Verify with a fresh session that the harness can discover and invoke the skill without copying user config.

## Shape B: Documentation-Only Wiring

Use this when the harness can read files or prompts but has no durable skill asset model.

- Keep `~/.agents/skills/` or the repository docs as the source of truth.
- Provide a short reference explaining how that harness should read the needed `SKILL.md`.
- Do not claim first-class support; describe it as a manual fallback.
- Prefer existing oma commands for mechanical state and verification so the fallback does not fork behavior.

## Shape C: Not Supportable Yet

Use this when the harness would require editing user-owned files, injecting hidden startup text, or maintaining a separate copy of skill bodies.

- Document the missing harness capability.
- Do not add a port that works only by mutating personal config.
- Do not add a copied skill tree that will drift from `assets/skills/`.
- Revisit only when the harness gains an install surface that can carry the mapping or context itself.

## Porting Checklist

- The shared skill body still names actions rather than harness tools.
- User-owned files are not edited by the port.
- Any bootstrap or mapping is shipped by the harness's own install artifact, or the limitation is documented.
- The port has a smoke test proving a fresh session can find the intended skill or documented fallback.
- The port does not add resident context for users who did not install or opt into it.
