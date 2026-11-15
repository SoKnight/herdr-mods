// Reading a space's checkout from Git, with no view state.
import { exec } from "herdr";

const basename = (path) => path.replace(/\/+$/, "").split("/").pop();

// The first terminal defines a space's checkout, not whichever pane is focused.
// Pass a panes() snapshot: each API call copies the full pane list.
export function spaceFolder(space, paneList) {
  const tabIDs = new Set((space.tabs ?? []).map((tab) => tab.id));
  return paneList.find((pane) => tabIDs.has(pane.tab) && pane.cwd)?.cwd || space.folder;
}

export const checkoutKey = (space, paneList) =>
  JSON.stringify([space.machine ?? null, spaceFolder(space, paneList)]);

// Git's "no repository here". A folder another Windows user owns (dubious ownership) is
// one Git won't read either, so it counts the same rather than as an endless failure.
export const notARepository = (stderr) => /not a git repository|dubious ownership/i.test(stderr ?? "");

// Branch, ahead/behind and repository, or null if this isn't a repository.
// Throws when Git couldn't be asked, so the caller keeps what it knew.
export async function readCheckout(cwd, on) {
  // Ref-only reads: no worktree scan, no index or fsmonitor touched, so our own reads
  // never wake the Git-folder watch into a refresh loop.
  const git = (args) => exec("git", args, { cwd, on });
  const [top, common] = await Promise.all([
    git(["rev-parse", "--path-format=absolute", "--show-toplevel"]),
    git(["rev-parse", "--path-format=absolute", "--git-common-dir"]),
  ]);
  if (top.code !== 0 || common.code !== 0) {
    if (notARepository(top.code !== 0 ? top.stderr : common.stderr)) return null;
    throw new Error("Cannot read repository metadata");
  }
  const [head, counts] = await Promise.all([
    git(["symbolic-ref", "-q", "HEAD"]),
    git(["rev-list", "--left-right", "--count", "HEAD...@{upstream}"]),
  ]);

  // The repo's own .git folder names it; a worktree's top differs from it.
  const repo = common.stdout.trim();
  const [ahead, behind] = counts.code === 0 ? counts.stdout.trim().split(/\s+/).map(Number) : [0, 0];
  return {
    name: repo.endsWith("/.git")
      ? basename(repo.slice(0, -5))
      : basename(repo).replace(/\.git$/, ""),
    branch: head.code === 0 ? head.stdout.trim().replace(/^refs\/heads\//, "") : "(detached)",
    ahead: ahead || 0,
    behind: behind || 0,
    repo: JSON.stringify([on, repo]),
    linked: repo !== `${top.stdout.trim()}/.git`,
  };
}
