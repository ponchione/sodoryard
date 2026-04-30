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

      <div className="w-full max-w-2xl">
        <div
          data-augmented-ui="tl-clip tr-clip bl-clip br-clip both"
          className="flex gap-2 p-1"
          style={{
            "--aug-tl": "15px",
            "--aug-tr": "15px",
            "--aug-bl": "15px",
            "--aug-br": "15px",
            "--aug-border-all": "2px",
            "--aug-border-bg": "#00e5ff",
            "--aug-inlay-all": "4px",
            "--aug-inlay-bg": "#0d1520",
          } as React.CSSProperties}
        >
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
            data-augmented-ui="tl-clip br-clip border"
            className="self-end bg-primary px-4 py-2 text-xs font-bold uppercase tracking-widest text-primary-foreground hover:bg-primary/80"
            style={{
              "--aug-tl": "6px",
              "--aug-br": "6px",
              "--aug-border-all": "1px",
              "--aug-border-bg": "#00e5ff",
            } as React.CSSProperties}
          >
            Send
          </Button>
        </div>
      </div>
    </div>
  );
}
