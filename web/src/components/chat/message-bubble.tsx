import type { CSSProperties } from "react";
import { ThinkingBlock } from "@/components/chat/thinking-block";
import { ToolCallCard } from "@/components/chat/tool-call-card";
import { MarkdownContent } from "@/components/chat/markdown-content";
import type { ChatMessage, ContentBlock } from "@/hooks/use-conversation";
import { getDisplayBlocks } from "@/lib/tool-transcript";

function BlockRenderer({ block, streaming }: { block: ContentBlock; streaming: boolean }) {
  switch (block.kind) {
    case "thinking":
      return <ThinkingBlock block={block} />;
    case "tool_call":
      return <ToolCallCard block={block} />;
    case "text":
      return (
        <div>
          <MarkdownContent content={block.text} />
          {streaming && (
            <span className="ml-0.5 inline-block h-4 w-1.5 bg-primary pulse-glow" />
          )}
        </div>
      );
  }
}

export function MessageBubble({
  message,
  streaming = false,
}: {
  message: ChatMessage;
  streaming?: boolean;
}) {
  const isUser = message.role === "user";
  const isSystem = message.role === "system";
  const isCompressed = message.isCompressed || message.isSummary;
  const displayBlocks = getDisplayBlocks(message.blocks);

  if (isSystem) {
    return (
      <div className="flex justify-center">
        <div className="max-w-[85%] border border-dashed border-[#ffab00]/40 bg-muted/30 px-4 py-2 text-xs text-muted-foreground italic">
          {isCompressed && (
            <span className="mr-1.5 inline-block bg-muted-foreground/20 px-1 py-0.5 text-[10px] font-medium not-italic">
              compressed
            </span>
          )}
          {message.content}
        </div>
      </div>
    );
  }

  if (!isUser && displayBlocks.length === 0) {
    return null;
  }

  if (isUser) {
    return (
      <div className="flex justify-end">
        <div
          data-augmented-ui="br-clip border"
          className="max-w-[85%] whitespace-pre-wrap bg-primary/10 px-4 py-2.5 text-sm text-foreground"
          style={{
            "--aug-br": "12px",
            "--aug-border-all": "1px",
            "--aug-border-bg": "#00e5ff60",
          } as CSSProperties}
        >
          {isCompressed && (
            <span className="mr-1.5 inline-block bg-muted-foreground/20 px-1 py-0.5 text-[10px] font-medium not-italic">
              compressed
            </span>
          )}
          {message.content}
        </div>
      </div>
    );
  }

  return (
    <div className="flex justify-start">
      <div
        data-augmented-ui="tl-clip border"
        className={`max-w-[85%] px-4 py-2.5 text-sm ${
          isCompressed
            ? "bg-muted/50 text-muted-foreground border border-dashed border-[#ffab00]/40"
            : "bg-muted text-foreground"
        }`}
        style={{
          "--aug-tl": "12px",
          "--aug-border-all": "1px",
          "--aug-border-bg": "#00e67640",
        } as CSSProperties}
      >
        {isCompressed && (
          <span className="mb-1.5 inline-block bg-muted-foreground/20 px-1 py-0.5 text-[10px] font-medium">
            compressed
          </span>
        )}
        {displayBlocks.map((block, i) => {
          const isLastBlock = i === displayBlocks.length - 1;
          return (
            <div key={i} data-augmented-ui-reset>
              <BlockRenderer block={block} streaming={streaming && isLastBlock} />
            </div>
          );
        })}
      </div>
    </div>
  );
}
