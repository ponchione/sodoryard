import type { CSSProperties, KeyboardEvent } from "react";
import { Button } from "@/components/ui/button";

export function ConversationComposer({
  input,
  isStreaming,
  onInputChange,
  onSend,
  onCancel,
}: {
  input: string;
  isStreaming: boolean;
  onInputChange: (value: string) => void;
  onSend: () => void;
  onCancel: () => void;
}) {
  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      onSend();
    }
  };

  return (
    <div className="border-t border-border p-4">
      <div className="mx-auto flex max-w-3xl gap-2">
        <div
          data-augmented-ui="tl-clip br-clip border"
          className="flex flex-1"
          style={{
            "--aug-tl": "10px",
            "--aug-br": "10px",
            "--aug-border-all": "1px",
            "--aug-border-bg": "#00e5ff60",
          } as CSSProperties}
        >
          <textarea
            value={input}
            onChange={(e) => onInputChange(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Type a message… (Enter to send, Shift+Enter for newline)"
            className="flex-1 resize-none border-0 bg-input px-3 py-2 text-sm outline-none placeholder:text-muted-foreground"
            rows={1}
            disabled={isStreaming}
            autoFocus
          />
        </div>
        {isStreaming ? (
          <Button
            variant="destructive"
            onClick={onCancel}
            data-augmented-ui="tl-clip br-clip border"
            className="border-0 bg-destructive/20 text-destructive hover:bg-destructive/30"
            style={{
              "--aug-tl": "6px",
              "--aug-br": "6px",
              "--aug-border-all": "1px",
              "--aug-border-bg": "#ff1744",
            } as CSSProperties}
          >
            Cancel
          </Button>
        ) : (
          <Button
            onClick={onSend}
            disabled={!input.trim()}
            data-augmented-ui="tl-clip br-clip border"
            className="border-0 bg-primary text-primary-foreground hover:bg-primary/80"
            style={{
              "--aug-tl": "6px",
              "--aug-br": "6px",
              "--aug-border-all": "1px",
              "--aug-border-bg": "#00e5ff",
            } as CSSProperties}
          >
            Send
          </Button>
        )}
      </div>
    </div>
  );
}
