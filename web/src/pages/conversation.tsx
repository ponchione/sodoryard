import { useParams } from "react-router-dom";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";

export function ConversationPage() {
  const { id } = useParams<{ id: string }>();

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      {/* Message area */}
      <ScrollArea className="flex-1 p-4">
        <div className="mx-auto max-w-3xl">
          <p className="py-12 text-center text-muted-foreground">
            Conversation {id} — messages will appear here
          </p>
        </div>
      </ScrollArea>

      {/* Input area */}
      <div className="border-t border-border p-4">
        <div className="mx-auto flex max-w-3xl gap-2">
          <Input
            placeholder="Type a message..."
            className="flex-1"
            autoFocus
          />
          <Button>Send</Button>
        </div>
      </div>
    </div>
  );
}
