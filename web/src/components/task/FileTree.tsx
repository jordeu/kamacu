import { useMemo, useState, type ReactNode } from "react";
import { Check, ChevronRight } from "lucide-react";
import { cn } from "@/lib/utils";
import type { DiffFile } from "@/api/diffs";
import { CommitStateMarkers } from "@/components/task/CommitStateMarkers";

/**
 * The left "Files changed" tree (DIFF-01): a nested, path-compressed,
 * folders-default-expanded directory tree derived purely client-side from the
 * already-sorted flat `files` list. It is pure presentation — clicking a leaf
 * is SCROLL-ONLY (`onSelect(path)`); it never mutates the file's collapse or
 * Viewed state (UI-SPEC Decision 1, deterministic). Row visuals mirror the
 * DiffFileSection header language: `font-mono text-sm truncate` filename,
 * `text-xs tabular-nums` green/red `+add −del` (`−` = U+2212), the established
 * `hover:bg-[#27272a]` + `focus-visible:ring-blue-500` interaction literals,
 * and — for the currently-scrolled file — `bg-accent` + a 2px blue-500 left
 * border. Viewed leaves dim to `muted-foreground` and gain a quiet Check glyph.
 *
 * The tree is nested-only, fixed-width, not resizable (D-07); the pane chrome
 * (w-72, border, overflow) lives in DiffTab. This component only renders rows.
 */

/** A node in the client-built file tree. Dirs may be path-compressed. */
type TreeNode =
  | { type: "dir"; name: string; path: string; children: TreeNode[] }
  | { type: "file"; name: string; path: string; file: DiffFile };

/** Mutable builder scaffold, converted to immutable TreeNodes on finalize. */
interface DirBuilder {
  name: string;
  path: string;
  dirs: Map<string, DirBuilder>;
  files: { type: "file"; name: string; path: string; file: DiffFile }[];
}

/**
 * Pure transform (modelled on the derive-from-cache shape of `applyMove`): split
 * each `file.path` on "/", create dir builders as needed, then path-compress
 * single-child dir chains GitHub-style (e.g. `internal/api`) and sort
 * dirs-before-files alphabetically (matching the server path sort). No side
 * effects — safe to run inside `useMemo`.
 */
function buildTree(files: DiffFile[]): TreeNode[] {
  const root: DirBuilder = {
    name: "",
    path: "",
    dirs: new Map(),
    files: [],
  };

  for (const file of files) {
    const segments = file.path.split("/");
    const fileName = segments.pop() ?? file.path;
    let cursor = root;
    let accum = "";
    for (const seg of segments) {
      accum = accum ? `${accum}/${seg}` : seg;
      let next = cursor.dirs.get(seg);
      if (!next) {
        next = { name: seg, path: accum, dirs: new Map(), files: [] };
        cursor.dirs.set(seg, next);
      }
      cursor = next;
    }
    cursor.files.push({
      type: "file",
      name: fileName,
      path: file.path,
      file,
    });
  }

  return finalizeChildren(root);
}

/** Sort a dir's children dirs-before-files, each alphabetical, then emit. */
function finalizeChildren(dir: DirBuilder): TreeNode[] {
  const dirNames = [...dir.dirs.keys()].sort((a, b) => a.localeCompare(b));
  const dirs = dirNames.map((name) => compressDir(dir.dirs.get(name)!));
  const files = [...dir.files].sort((a, b) => a.name.localeCompare(b.name));
  return [...dirs, ...files];
}

/**
 * Fold single-child directory chains into one row (GitHub path compression):
 * while a dir has exactly one child dir and no direct files, merge its name
 * into "name/childName" and descend. Stop at the first dir with >1 child or a
 * direct file.
 */
function compressDir(dir: DirBuilder): TreeNode {
  let name = dir.name;
  let current = dir;
  while (current.dirs.size === 1 && current.files.length === 0) {
    const child = [...current.dirs.values()][0];
    name = `${name}/${child.name}`;
    current = child;
  }
  return {
    type: "dir",
    name,
    // The compressed dir's identity/toggle key is the deepest folded path.
    path: current.path,
    children: finalizeChildren(current),
  };
}

export function FileTree({
  files,
  activePath,
  onSelect,
}: {
  files: DiffFile[];
  activePath: string | null;
  onSelect: (path: string) => void;
}) {
  // `files` is a stable reference between renders (TanStack structural sharing);
  // an optimistic Viewed toggle produces a NEW files array, so the tree rebuilds
  // and leaf rows reflect the new viewed/counts without stale nodes.
  const tree = useMemo(() => buildTree(files), [files]);

  // Tree-only open state (unrelated to per-file diff collapse). Folders default
  // expanded (D-06): a dir is open unless its path is in this collapsed set.
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const toggleDir = (path: string) => {
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return next;
    });
  };

  const renderNodes = (nodes: TreeNode[], depth: number): ReactNode =>
    nodes.map((node) => {
      const indent = { paddingLeft: 8 + depth * 12 };
      if (node.type === "dir") {
        const open = !collapsed.has(node.path);
        return (
          <div key={`dir:${node.path}`}>
            <button
              type="button"
              onClick={() => toggleDir(node.path)}
              aria-expanded={open}
              style={indent}
              className="flex w-full items-center gap-1 rounded-sm py-1 pr-2 text-left text-sm text-muted-foreground outline-none hover:bg-[#27272a] focus-visible:ring-2 focus-visible:ring-blue-500"
            >
              <ChevronRight
                className={cn(
                  "size-4 shrink-0 transition-transform",
                  open && "rotate-90",
                )}
              />
              <span className="min-w-0 flex-1 truncate">{node.name}</span>
            </button>
            {open && renderNodes(node.children, depth + 1)}
          </div>
        );
      }

      const file = node.file;
      const selected = file.path === activePath;
      return (
        <button
          key={`file:${node.path}`}
          type="button"
          onClick={() => onSelect(file.path)}
          aria-current={selected ? "true" : undefined}
          style={indent}
          className={cn(
            "flex min-h-7 w-full items-center gap-2 rounded-sm border-l-2 border-transparent py-1 pr-2 text-left outline-none hover:bg-[#27272a] focus-visible:ring-2 focus-visible:ring-blue-500",
            selected && "border-blue-500 bg-accent",
          )}
          >
            <CommitStateMarkers
              uncommitted={file.uncommitted}
              unpushed={file.unpushed}
            />
            <span
              className={cn(
                "min-w-0 flex-1 truncate font-mono text-sm",
                file.viewed && "text-muted-foreground",
              )}
            >
              {node.name}
            </span>
          {file.viewed && (
            <Check className="size-3 shrink-0 text-muted-foreground" />
          )}
          {!file.binary && (
            <span
              className={cn(
                "flex shrink-0 items-center gap-1 text-xs tabular-nums",
                file.viewed && "text-muted-foreground",
              )}
            >
              <span className={file.viewed ? undefined : "text-green-400"}>
                {`+${file.additions ?? 0}`}
              </span>
              <span className={file.viewed ? undefined : "text-red-400"}>
                {`−${file.deletions ?? 0}`}
              </span>
            </span>
          )}
        </button>
      );
    });

  return (
    <nav aria-label="Changed files" className="py-1">
      {renderNodes(tree, 0)}
    </nav>
  );
}
