import type { KeyboardEvent } from "react";
import { Button } from "@/components/ui/button";

export function ConversationComposer({
  input,
  isStreaming,
  canSend,
  onInputChange,
  onSend,
  onCancel,
}: {
  input: string;
  isStreaming: boolean;
  canSend: boolean;
  onInputChange: (value: string) => void;
  onSend: () => void;
  onCancel: () => void;
}) {
  const sendDisabled = isStreaming || !canSend || !input.trim();

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      if (!sendDisabled) {
        onSend();
      }
    }
  };

  return (
    <div className="border-t border-border p-4">
      <div className="mx-auto flex w-full max-w-4xl gap-2">
        <div className="flex flex-1 border border-border bg-input focus-within:border-primary">
          <textarea
            value={input}
            onChange={(e) => onInputChange(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Type a message… (Enter to send, Shift+Enter for newline)"
            className="flex-1 resize-none border-0 bg-transparent px-3 py-2 text-sm outline-none placeholder:text-muted-foreground"
            rows={1}
            disabled={isStreaming}
            autoFocus
          />
        </div>
        {isStreaming ? (
          <Button
            variant="destructive"
            onClick={onCancel}
            className="border border-destructive/40 bg-destructive/20 text-destructive hover:bg-destructive/30"
          >
            Cancel
          </Button>
        ) : (
          <Button
            onClick={onSend}
            disabled={sendDisabled}
            className="border border-primary bg-primary px-4 text-primary-foreground hover:bg-primary/80"
          >
            Send
          </Button>
        )}
      </div>
    </div>
  );
}
