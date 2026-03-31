import { Button } from "@/components/ui/button";
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { useNavigate } from "react-router-dom";

export function ConversationListPage() {
  const navigate = useNavigate();

  const handleNewConversation = () => {
    // TODO: Create conversation via API, then navigate to /c/:id
    void navigate;
  };

  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-6 p-8">
      <div className="text-center">
        <h1 className="text-3xl font-bold tracking-tight">sirtopham</h1>
        <p className="mt-2 text-muted-foreground">AI coding assistant</p>
      </div>

      <Card className="w-full max-w-md">
        <CardHeader className="text-center">
          <CardTitle>Start a conversation</CardTitle>
          <CardDescription>
            Ask a question about your codebase or request a change.
          </CardDescription>
        </CardHeader>
        <div className="p-6 pt-0">
          <Button className="w-full" onClick={handleNewConversation}>
            New conversation
          </Button>
        </div>
      </Card>
    </div>
  );
}
