import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { Prism as SyntaxHighlighter } from "react-syntax-highlighter";
import { oneDark } from "react-syntax-highlighter/dist/esm/styles/prism";
import type { Components } from "react-markdown";

interface MarkdownContentProps {
  content: string;
}

const components: Components = {
  code(props) {
    const { children, className, ...rest } = props;
    const match = /language-(\w+)/.exec(className || "");
    const code = String(children).replace(/\n$/, "");

    if (match) {
      return (
        <SyntaxHighlighter
          style={oneDark}
          language={match[1]}
          PreTag="div"
          className="!rounded-md !text-xs !my-2"
        >
          {code}
        </SyntaxHighlighter>
      );
    }

    // Inline code.
    return (
      <code
        className="rounded bg-muted-foreground/15 px-1 py-0.5 text-xs font-mono"
        {...rest}
      >
        {children}
      </code>
    );
  },
  pre(props) {
    // Let the code component handle syntax highlighting inside pre.
    return <>{props.children}</>;
  },
  p(props) {
    return <p className="mb-2 last:mb-0" {...props} />;
  },
  ul(props) {
    return <ul className="mb-2 ml-4 list-disc last:mb-0" {...props} />;
  },
  ol(props) {
    return <ol className="mb-2 ml-4 list-decimal last:mb-0" {...props} />;
  },
  li(props) {
    return <li className="mb-0.5" {...props} />;
  },
  a(props) {
    return (
      <a
        className="text-primary underline underline-offset-2 hover:text-primary/80"
        target="_blank"
        rel="noopener noreferrer"
        {...props}
      />
    );
  },
  blockquote(props) {
    return (
      <blockquote
        className="mb-2 border-l-2 border-border pl-3 text-muted-foreground italic last:mb-0"
        {...props}
      />
    );
  },
  table(props) {
    return (
      <div className="mb-2 overflow-x-auto last:mb-0">
        <table className="min-w-full text-xs border-collapse" {...props} />
      </div>
    );
  },
  th(props) {
    return (
      <th
        className="border border-border bg-muted px-2 py-1 text-left font-semibold"
        {...props}
      />
    );
  },
  td(props) {
    return <td className="border border-border px-2 py-1" {...props} />;
  },
  h1(props) {
    return <h1 className="mb-2 text-lg font-bold" {...props} />;
  },
  h2(props) {
    return <h2 className="mb-2 text-base font-bold" {...props} />;
  },
  h3(props) {
    return <h3 className="mb-1.5 text-sm font-bold" {...props} />;
  },
  hr() {
    return <hr className="my-3 border-border" />;
  },
};

export function MarkdownContent({ content }: MarkdownContentProps) {
  return (
    <div className="prose-sm max-w-none">
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={components}>
        {content}
      </ReactMarkdown>
    </div>
  );
}
