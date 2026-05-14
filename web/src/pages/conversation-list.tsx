import { useState, type KeyboardEvent } from "react";
import { useNavigate } from "react-router-dom";
import { Button } from "@/components/ui/button";

export function ConversationListPage() {
  const navigate = useNavigate();
  const [input, setInput] = useState("");

  const handleSend = () => {
    const text = input.trim();
    if (!text) return;
    navigate("/c/new", { state: { initialMessage: text } });
  };

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  };

  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-6 p-8">
      <div className="text-center">
        <h1 className="text-3xl font-bold uppercase tracking-[0.25em] text-primary text-glow-cyan">
          Sodoryard
        </h1>
        <p className="mt-2 text-sm text-muted-foreground">
          // AI coding assistant
        </p>
      </div>

      <div className="w-full max-w-4xl">
        <div className="flex gap-2 border border-border bg-input p-1 focus-within:border-primary">
          <textarea
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Ask a question about your codebase…"
            className="flex-1 resize-none border-0 bg-transparent px-3 py-2.5 text-sm text-foreground outline-none placeholder:text-muted-foreground"
            rows={1}
            autoFocus
          />
          <Button
            onClick={handleSend}
            disabled={!input.trim()}
            className="self-end border border-primary bg-primary px-4 py-2 text-xs font-bold uppercase tracking-widest text-primary-foreground hover:bg-primary/80"
          >
            Send
          </Button>
        </div>
      </div>
    </div>
  );
}
