import type { ComponentProps } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

export function InboxText({ text, paragraphs = false }: { text: string; paragraphs?: boolean }) {
  return <ReactMarkdown skipHtml remarkPlugins={[remarkGfm]} components={{
    p: ({ children }) => paragraphs ? <p>{children}</p> : <span className="block">{children}</span>,
    img: () => null,
    a: ({ href, children }) => {
      const match = href?.match(/^https?:\/\/github\.com\/[^/]+\/([^/]+)\/(?:pull|issues)\/(\d+)(?:[/?#].*)?$/i);
      return <a href={href} target="_blank" rel="noopener noreferrer" title={href} onClick={(event) => event.stopPropagation()} className="pointer-events-auto text-blue-400 underline underline-offset-2 hover:text-blue-300">{match ? `${match[1]}#${match[2]}` : children}</a>;
    },
  }}>{text}</ReactMarkdown>;
}

// Keep links and the action button as siblings so following a link never submits
// an answer or opens the message dialog.
export function InboxTextAction({ text, className, ...props }: { text: string } & ComponentProps<"button">) {
  return <div className={`relative ${className ?? ""} ${props.disabled ? "opacity-50" : ""}`}>
    <button {...props} type="button" aria-label={props["aria-label"] ?? text} className="absolute inset-0 rounded-[inherit] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-default" />
    <div className="pointer-events-none relative min-w-0"><InboxText text={text} /></div>
  </div>;
}
