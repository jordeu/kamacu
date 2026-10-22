import { useEffect, useRef, useState } from "react";
import type { Artifact } from "@/api/artifacts";
import { Button } from "@/components/ui/button";

// The displayed revision is intentionally independent of the polled revision.
// Hidden tabs remain mounted; unopened tabs do not start a document or scripts.
export function ArtifactTab({ artifact, active }: { artifact: Artifact; active: boolean }) {
  const [revision, setRevision] = useState<string | null>(null);
  const [attempt, setAttempt] = useState(0);
  if (active && revision === null) setRevision(artifact.revision);

  function reload() {
    setRevision(artifact.revision);
    setAttempt((value) => value + 1);
  }

  return (
    <div className="flex h-full min-h-[320px] flex-col gap-2">
      {revision !== null && revision !== artifact.revision && (
        <div role="status" className="flex items-center justify-between gap-3 text-sm text-muted-foreground">
          <span>Updated</span>
          <Button variant="outline" size="sm" onClick={reload}>Reload</Button>
        </div>
      )}
      {revision && (
        <ArtifactPreview
          key={`${revision}:${attempt}`}
          title={artifact.title}
          url={`/api/artifact-content/${artifact.id}/${revision}/index.html`}
          onRetry={reload}
        />
      )}
    </div>
  );
}

function ArtifactPreview({ title, url, onRetry }: {
  title: string; url: string; onRetry: () => void;
}) {
  const [token] = useState(() => crypto.randomUUID());
  const [status, setStatus] = useState<"checking" | "loading" | "loaded">("checking");
  const [error, setError] = useState<string | null>(null);
  const iframe = useRef<HTMLIFrameElement>(null);
  const progress = useRef({ verified: false, loaded: false });
  const timeout = useRef<ReturnType<typeof setTimeout> | null>(null);

  function finishLoading() {
    if (progress.current.verified && progress.current.loaded) {
      if (timeout.current !== null) clearTimeout(timeout.current);
      setStatus("loaded");
    }
  }

  useEffect(() => {
    const controller = new AbortController();
    let disposed = false;
    const fail = (message: string) => {
      if (!disposed) setError(message);
      if (timeout.current !== null) clearTimeout(timeout.current);
    };
    const receive = (event: MessageEvent) => {
      if (event.source !== iframe.current?.contentWindow ||
          event.data?.kind !== "kamacu-artifact-preview" || event.data?.token !== token) return;
      if (event.data.status === "unsupported") {
        fail("Your browser cannot enforce this preview’s network restrictions. Use a browser with Connection Allowlists support enabled.");
      } else if (event.data.status === "ready") {
        progress.current.verified = true;
        if (progress.current.loaded) {
          if (timeout.current !== null) clearTimeout(timeout.current);
          setStatus("loaded");
        }
      }
    };
    window.addEventListener("message", receive);
    timeout.current = setTimeout(() => {
      controller.abort();
      fail("The preview took too long to load. Check your connection and retry.");
    }, 15_000);
    // Iframe load/error events do not expose HTTP failures. Check the actual
    // resource first; the trusted renderer handshake also covers a subsequent
    // failed navigation between this request and the iframe request.
    fetch(url, { method: "HEAD", cache: "no-store", signal: controller.signal })
      .then((response) => {
        if (!response.ok) throw new Error(response.status === 404
          ? "The preview is no longer available. Retry to check again."
          : "The preview could not be loaded. Check your connection and retry.");
        if (!disposed) setStatus("loading");
      })
      .catch((reason: unknown) => {
        if (!controller.signal.aborted) fail(reason instanceof Error && !(reason instanceof TypeError)
          ? reason.message : "The preview could not be loaded. Check your connection and retry.");
      });
    return () => {
      disposed = true;
      controller.abort();
      if (timeout.current !== null) clearTimeout(timeout.current);
      window.removeEventListener("message", receive);
    };
  }, [url, token]);

  return <>
    {error ? (
      <div className="flex flex-wrap items-center gap-3">
        <p role="alert" className="text-sm text-destructive">{error}</p>
        <Button variant="outline" size="sm" onClick={onRetry}>Retry</Button>
      </div>
    ) : <>
      {status !== "loaded" && <p role="status" className="text-sm text-muted-foreground">Loading preview…</p>}
      {status !== "checking" && <iframe
        ref={iframe}
        title={title}
        src={`${url}?preview_token=${encodeURIComponent(token)}`}
        sandbox="allow-scripts"
        referrerPolicy="no-referrer"
        className="min-h-[320px] w-full flex-1 border-0 bg-white"
        onLoad={() => { progress.current.loaded = true; finishLoading(); }}
        onError={() => {
          if (timeout.current !== null) clearTimeout(timeout.current);
          setError("The preview could not be loaded. Check your connection and retry.");
        }}
      />}
    </>}
  </>;
}
