import { useState, type KeyboardEvent } from "react";
import { useNavigate } from "react-router-dom";
import { Button } from "@/components/ui/button";

export function ConversationListPage() {
  const navigate = useNavigate();
  const [input, setInput] = useState("");

  const handleSend = () => {
    const text = input.trim();
    if (!text) return;
    // Navigate to a new conversation page with the initial message as state.
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
        <h1 className="text-3xl font-bold tracking-tight">sirtopham</h1>
        <p className="mt-2 text-muted-foreground">AI coding assistant</p>
      </div>

      <div className="w-full max-w-2xl">
        <div className="flex gap-2">
          <textarea
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Ask a question about your codebase…"
            className="flex-1 resize-none rounded-lg border border-input bg-background px-3 py-2.5 text-sm outline-none ring-ring/50 placeholder:text-muted-foreground focus-visible:ring-2"
            rows={1}
            autoFocus
          />
          <Button onClick={handleSend} disabled={!input.trim()}>
            Send
          </Button>
        </div>
      </div>
    </div>
  );
}
