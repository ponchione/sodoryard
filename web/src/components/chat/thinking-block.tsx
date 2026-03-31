import { useState } from "react";
import type { ThinkingBlock as ThinkingBlockData } from "@/hooks/use-conversation";

interface ThinkingBlockProps {
  block: ThinkingBlockData;
}

export function ThinkingBlock({ block }: ThinkingBlockProps) {
  const [open, setOpen] = useState(false);

  return (
    <div className="my-1.5">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground transition-colors"
      >
        <svg
          className={`h-3 w-3 transition-transform ${open ? "rotate-90" : ""}`}
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          strokeWidth={2}
        >
          <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
        </svg>
        <span className="italic">
          {block.done ? "Thought" : "Thinking"}
          {!block.done && (
            <span className="ml-1 inline-block h-2 w-2 animate-pulse rounded-full bg-muted-foreground" />
          )}
        </span>
        {block.text.length > 0 && (
          <span className="text-muted-foreground/60">
            ({block.text.length} chars)
          </span>
        )}
      </button>
      {open && (
        <div className="mt-1.5 ml-4 rounded-md border border-border/50 bg-muted/30 px-3 py-2 text-xs text-muted-foreground whitespace-pre-wrap font-mono leading-relaxed max-h-64 overflow-y-auto">
          {block.text || "…"}
          {!block.done && (
            <span className="ml-0.5 inline-block h-3 w-0.5 animate-pulse bg-muted-foreground" />
          )}
        </div>
      )}
    </div>
  );
}
