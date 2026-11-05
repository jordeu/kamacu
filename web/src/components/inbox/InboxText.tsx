import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

export function InboxText({ text }: { text: string }) {
  return <ReactMarkdown skipHtml remarkPlugins={[remarkGfm]} components={{
    p: ({ children }) => <span className="block">{children}</span>,
    img: () => null,
    a: ({ href, children }) => {
      const match = href?.match(/^https?:\/\/github\.com\/[^/]+\/([^/]+)\/(?:pull|issues)\/(\d+)(?:[/?#].*)?$/i);
      return <a href={href} target="_blank" rel="noopener noreferrer" title={href} className="text-blue-400 underline underline-offset-2 hover:text-blue-300">{match ? `${match[1]}#${match[2]}` : children}</a>;
    },
  }}>{text}</ReactMarkdown>;
}
