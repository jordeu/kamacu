import { useMemo, useState } from "react";
import { FileText } from "lucide-react";
import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import { useTaskDiffContent, type DiffFile } from "@/api/diffs";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import {
  buildMarkdownDiff,
  type Block,
  type BlockRenderItem,
  type RemovedRun,
} from "@/lib/markdown-diff";
import { UnifiedDiffBody } from "@/components/task/UnifiedDiffBody";

/**
 * The Markdown viewer (MDV-01): an explicit "Read as document" control in the
 * file header (a SIBLING of the collapse trigger — button-in-button is invalid
 * HTML, same rule as the Viewed checkbox) that opens a dialog rendering the
 * NEW version of a changed .md as formatted prose with the diff marked in
 * place: added/changed blocks green-tinted with a green left bar; removed
 * content struck-through, red-tinted, interleaved at its original position so
 * the document still reads in order. FileTree row clicks stay SCROLL-ONLY
 * (UI-SPEC Decision 1) — this control is the only entry point.
 *
 * A "rendered / source" switch (MDV-02) falls back to the existing unified
 * diff via the same UnifiedDiffBody the file section renders. Loud diff color
 * stays inside the diff content (the rendered document / unified body); the
 * dialog chrome stays neutral popover/ring (05-UI-SPEC Layout & Color).
 */

// Loud-inside-quiet diff palette (05-UI-SPEC diff content palette, matching
// DiffFileSection's green-500/10 / red-500/10). [&>*]:my-2 compacts prose
// margins inside the tint so the block hugs its content.
const ADDED_BLOCK =
  "rounded-r-md border-l-2 border-green-500 bg-green-500/10 px-3 py-1 [&>*]:my-2";
const GHOST_BLOCK =
  "rounded-r-md border-l-2 border-red-500 bg-red-500/10 px-3 py-1 text-zinc-400 line-through decoration-red-400/60 [&>*]:my-2 [&_a]:no-underline";
const ROW_ADDED = "bg-green-500/10";
const ROW_GHOST = "bg-red-500/10 text-zinc-400 line-through";

/** Strip a list marker ("- ", "* ", "1. ", "1) ") for ghost <li> text. */
const RE_ITEM_MARK = /^\s*(?:[-*+]|\d{1,9}[.)])\s+/;

/** Split a GFM table row into trimmed cells (outer pipes dropped). */
function tableCells(line: string): string[] {
  let s = line.trim();
  if (s.startsWith("|")) s = s.slice(1);
  if (s.endsWith("|") && !s.endsWith("\\|")) s = s.slice(0, -1);
  return s.split("|").map((c) => c.trim());
}

/** Structural slice of a hast node — positions + properties are all the
 *  attribution reads (react-markdown keeps position info by default). */
interface HastEl {
  type: "element";
  tagName?: string;
  properties?: Record<string, unknown>;
  position?: {
    start?: { line?: number };
    end?: { line?: number };
  };
  children?: HastNode[];
}

type HastNode = HastEl | { type: string; [k: string]: unknown };

function isElement(n: HastNode | undefined): n is HastEl {
  return !!n && (n as HastEl).type === "element";
}

/**
 * Sub-block attribution as a REHYPE plugin (the architecture fix): running
 * inside react-markdown's hast pipeline means positions are ground truth and
 * ghost nodes are REAL tree members — a component-override splice cannot
 * work, because parent overrides receive children as not-yet-rendered
 * component elements (their computed props don't exist yet).
 *
 * The plugin walks the rendered block's hast tree and:
 *   - tags tr/li whose line span contains an added line (data-mdv="added");
 *   - splices ghost tr/li nodes (data-mdv="ghost", struck-through text)
 *     inside their container at the removed runs' anchor positions.
 * Components downstream only translate data-mdv to classes.
 */
function rehypeMdv(opts: {
  block: Block;
  offset: number;
  addedLines: Set<number>;
  ghostRuns: RemovedRun[];
}) {
  const { block, offset, addedLines, ghostRuns } = opts;
  const abs = (p: number) => block.start + p - offset - 1;
  const spanAdded = (node: HastEl) => {
    const s = node.position?.start?.line;
    const e = node.position?.end?.line ?? s;
    if (s === undefined || e === undefined) return false;
    for (let p = s; p <= e; p++) {
      if (addedLines.has(abs(p))) return true;
    }
    return false;
  };

  const ghostNode = (kind: "row" | "item", line: string): HastEl => {
    if (kind === "row") {
      return {
        type: "element",
        tagName: "tr",
        properties: { "data-mdv": "ghost" },
        children: tableCells(line).map((c) => ({
          type: "element",
          tagName: "td",
          properties: {},
          children: [{ type: "text", value: c }],
        })),
      };
    }
    return {
      type: "element",
      tagName: "li",
      properties: { "data-mdv": "ghost" },
      children: [{ type: "text", value: line.replace(RE_ITEM_MARK, "") }],
    };
  };

  return (tree: HastNode) => {
    const walk = (node: HastNode) => {
      // The root itself is type "root", not an element — always descend.
      const el = isElement(node) ? node : undefined;
      const tag = el?.tagName ?? "";
      const kids = ((node as { children?: HastNode[] }).children ?? []).slice();

      // Containers that own ghostable rows/items: splice runs by anchor.
      if (tag === "tbody" || tag === "thead" || tag === "ul" || tag === "ol") {
        const kind = tag === "tbody" || tag === "thead" ? "row" : "item";
        const runs = [...ghostRuns].sort((a, b) => a.anchor - b.anchor);
        let ri = 0;
        const next: HastNode[] = [];
        const flush = (upTo: number | undefined) => {
          while (ri < runs.length && (upTo === undefined || runs[ri].anchor <= upTo)) {
            for (const line of runs[ri].lines) {
              next.push(ghostNode(kind, line));
            }
            ri++;
          }
        };
        for (const child of kids) {
          // Container children interleave whitespace text nodes (no
          // position); only ELEMENT children with a position participate in
          // anchor ordering — anything else passes through unflushed, or it
          // would trigger the unconditional trailing flush early.
          const startLine = isElement(child)
            ? child.position?.start?.line
            : undefined;
          if (typeof startLine === "number") flush(abs(startLine));
          next.push(child);
        }
        // Trailing runs (anchored past the last child) append at the end of
        // CONTENT containers only — thead would otherwise collect every run
        // as duplicate header ghosts.
        if (tag !== "thead") flush(undefined);
        (node as { children?: HastNode[] }).children = next;
      }

      // Tag rows/items whose span contains an added line.
      if ((tag === "tr" || tag === "li") && el) {
        if (spanAdded(el)) {
          el.properties = { ...el.properties, "data-mdv": "added" };
        }
      }

      for (const child of kids) walk(child);
    };
    walk(tree);
  };
}

export function MarkdownDiffDialog({
  taskId,
  file,
}: {
  taskId: number;
  file: DiffFile;
}) {
  const [open, setOpen] = useState(false);
  // Fetch only while the dialog is open (MDV-01 data source gating).
  const { data, isPending, isError, error, refetch } = useTaskDiffContent(
    taskId,
    file.path,
    open,
  );

  const statusSuffix =
    file.status === "new"
      ? "new"
      : file.status === "deleted"
        ? "deleted"
        : null;
  const filename = file.path.split("/").pop() ?? file.path;

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <Tooltip>
        <TooltipTrigger asChild>
          <DialogTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={`Read ${filename} as a document`}
            >
              <FileText className="size-4" />
            </Button>
          </DialogTrigger>
        </TooltipTrigger>
        <TooltipContent>Read as document</TooltipContent>
      </Tooltip>
      <DialogContent className="flex h-[85vh] w-[calc(100%-2rem)] max-w-none flex-col gap-0 p-0 sm:max-w-3xl">
        <Tabs defaultValue="rendered" className="flex min-h-0 flex-1 flex-col gap-0">
          <div className="flex shrink-0 items-center gap-3 border-b border-border px-4 py-3 pr-12">
            <DialogTitle className="min-w-0 flex-1 truncate font-mono text-sm leading-none">
              {file.path}
              {statusSuffix && (
                <span className="ml-2 font-sans text-xs text-muted-foreground">
                  {statusSuffix}
                </span>
              )}
            </DialogTitle>
            <TabsList className="shrink-0 text-xs">
              <TabsTrigger value="rendered">Rendered</TabsTrigger>
              <TabsTrigger value="source">Source</TabsTrigger>
            </TabsList>
          </div>
          <DialogDescription className="sr-only">
            {`Markdown document rendered with the diff highlighted in place.`}
          </DialogDescription>
          <TabsContent
            value="rendered"
            className="min-h-0 flex-1 overflow-y-auto focus-visible:outline-none"
          >
            {isPending && !data ? (
              <p className="px-6 py-4 text-sm text-muted-foreground">
                {`Loading document…`}
              </p>
            ) : isError && !data ? (
              <div className="flex flex-col items-start gap-2 px-6 py-4">
                <p className="text-sm">{`Couldn't load the document.`}</p>
                <p className="text-xs text-muted-foreground">{error.message}</p>
                <Button variant="outline" size="sm" onClick={() => refetch()}>
                  Retry
                </Button>
              </div>
            ) : data ? (
              <div className="px-6 py-4">
                <RenderedDocument
                  newText={data.newText}
                  hunks={file.hunks}
                />
              </div>
            ) : null}
          </TabsContent>
          <TabsContent
            value="source"
            className="min-h-0 flex-1 overflow-y-auto focus-visible:outline-none"
          >
            <UnifiedDiffBody hunks={file.hunks} />
          </TabsContent>
        </Tabs>
      </DialogContent>
    </Dialog>
  );
}

/** The rendered view: the attributed block tree of the NEW text. */
function RenderedDocument({
  newText,
  hunks,
}: {
  newText: string | null;
  hunks: DiffFile["hunks"];
}) {
  const { items, refDefs, addedLines } = useMemo(
    () => buildMarkdownDiff(newText, hunks),
    [newText, hunks],
  );

  if (items.length === 0) {
    return <p className="text-sm text-muted-foreground">Empty document.</p>;
  }

  return (
    <div className="prose prose-invert prose-sm max-w-none prose-a:text-blue-500">
      {items.map((item, i) =>
        item.kind === "ghost" ? (
          <div key={i} className={GHOST_BLOCK}>
            {item.blocks.map((b, bi) => (
              <ReactMarkdown key={bi} remarkPlugins={[remarkGfm]}>
                {refDefs ? `${refDefs}\n${b.source}` : b.source}
              </ReactMarkdown>
            ))}
          </div>
        ) : (
          <DocumentBlock
            key={i}
            item={item}
            refDefs={refDefs}
            addedLines={addedLines}
          />
        ),
      )}
    </div>
  );
}

/**
 * One NEW-text block. Non-table/list blocks carry the whole-block green tint
 * when any of their lines was added; tables and lists skip the whole-block
 * tint in favor of exact row/item tinting via hast positions (below) — a
 * one-row table change reads as one green row, not a green table.
 */
function DocumentBlock({
  item,
  refDefs,
  addedLines,
}: {
  item: BlockRenderItem;
  refDefs: string;
  addedLines: Set<number>;
}) {
  const { block, added, ghostRuns } = item;
  // Reference definitions are prepended so [text][ref] links resolve across
  // per-block rendering; `offset` converts hast line numbers (1-based within
  // the render source) back to absolute NEW-text line numbers.
  const offset = refDefs ? refDefs.split("\n").length : 0;
  const source = offset ? `${refDefs}\n${block.source}` : block.source;
  const components = useMemo(() => blockComponents(), []);
  const rehypePlugins = useMemo<
    [typeof rehypeMdv, Parameters<typeof rehypeMdv>[0]][]
  >(
    () => [[rehypeMdv, { block, offset, addedLines, ghostRuns }]],
    [block, offset, addedLines, ghostRuns],
  );

  const markdown = (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      rehypePlugins={rehypePlugins}
      components={components}
    >
      {source}
    </ReactMarkdown>
  );

  const subRendered = block.type === "table" || block.type === "list";
  if (!added || subRendered) return markdown;
  return <div className={ADDED_BLOCK}>{markdown}</div>;
}

/**
 * Sub-block attribution runs as the rehypeMdv plugin (see below); these
 * component overrides are pure styling — they translate the data-mdv tags
 * the plugin left on tr/li nodes into the diff palette classes.
 */
function blockComponents(): Components {
  const cls = (node: HastEl | undefined) => {
    const mdv = node?.properties?.["data-mdv"];
    if (mdv === "ghost") return ROW_GHOST;
    if (mdv === "added") return ROW_ADDED;
    return undefined;
  };
  return {
    tr: ({ node, children, ...rest }) => (
      <tr {...rest} className={cls(node as HastEl) ?? undefined}>
        {children}
      </tr>
    ),
    li: ({ node, children, ...rest }) => (
      <li {...rest} className={cls(node as HastEl) ?? undefined}>
        {children}
      </li>
    ),
  };
}
