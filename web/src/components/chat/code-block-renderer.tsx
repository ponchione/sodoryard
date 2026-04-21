import { PrismLight as SyntaxHighlighter } from "react-syntax-highlighter";
import { oneDark } from "react-syntax-highlighter/dist/esm/styles/prism";
import bash from "react-syntax-highlighter/dist/esm/languages/prism/bash";
import diff from "react-syntax-highlighter/dist/esm/languages/prism/diff";
import go from "react-syntax-highlighter/dist/esm/languages/prism/go";
import javascript from "react-syntax-highlighter/dist/esm/languages/prism/javascript";
import json from "react-syntax-highlighter/dist/esm/languages/prism/json";
import jsx from "react-syntax-highlighter/dist/esm/languages/prism/jsx";
import markdown from "react-syntax-highlighter/dist/esm/languages/prism/markdown";
import python from "react-syntax-highlighter/dist/esm/languages/prism/python";
import sql from "react-syntax-highlighter/dist/esm/languages/prism/sql";
import tsx from "react-syntax-highlighter/dist/esm/languages/prism/tsx";
import typescript from "react-syntax-highlighter/dist/esm/languages/prism/typescript";
import yaml from "react-syntax-highlighter/dist/esm/languages/prism/yaml";

interface CodeBlockRendererProps {
  code: string;
  language: string;
}

const supportedLanguages = {
  bash,
  diff,
  go,
  javascript,
  json,
  jsx,
  markdown,
  python,
  sql,
  tsx,
  typescript,
  yaml,
} as const;

const languageAliases: Record<string, keyof typeof supportedLanguages> = {
  golang: "go",
  js: "javascript",
  md: "markdown",
  py: "python",
  shell: "bash",
  sh: "bash",
  ts: "typescript",
  yml: "yaml",
  zsh: "bash",
};

for (const [name, grammar] of Object.entries(supportedLanguages)) {
  SyntaxHighlighter.registerLanguage(name, grammar);
}

function normalizeLanguage(language: string): keyof typeof supportedLanguages | null {
  const normalized = language.trim().toLowerCase();
  if (!normalized) {
    return null;
  }
  if (normalized in supportedLanguages) {
    return normalized as keyof typeof supportedLanguages;
  }
  return languageAliases[normalized] ?? null;
}

function PlainCodeBlock({ code }: { code: string }) {
  return (
    <pre
      data-code-block-renderer="plain"
      className="my-2 overflow-x-auto rounded-md bg-[#282c34] p-3 text-xs text-slate-100"
    >
      <code>{code}</code>
    </pre>
  );
}

export function CodeBlockRenderer({ code, language }: CodeBlockRendererProps) {
  const normalizedLanguage = normalizeLanguage(language);
  if (!normalizedLanguage) {
    return <PlainCodeBlock code={code} />;
  }

  return (
    <div data-code-block-renderer="highlighted">
      <SyntaxHighlighter
        style={oneDark}
        language={normalizedLanguage}
        PreTag="div"
        className="!my-2 !rounded-md !text-xs"
      >
        {code}
      </SyntaxHighlighter>
    </div>
  );
}
