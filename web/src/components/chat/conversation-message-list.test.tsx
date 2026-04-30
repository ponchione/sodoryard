import { createRef } from "react";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { ChatMessage } from "@/hooks/use-conversation";
import { ConversationMessageList } from "./conversation-message-list";

function makeUserMessage(index: number): ChatMessage {
  return {
    role: "user",
    content: `user-${index}`,
    blocks: [],
  };
}

describe("ConversationMessageList", () => {
  it("renders only the latest bounded message window", () => {
    const messages = Array.from({ length: 205 }, (_, index) => makeUserMessage(index));

    render(
      <ConversationMessageList
        messages={messages}
        streamingText=""
        isStreaming={false}
        agentState={null}
        error={null}
        usage={null}
        messagesEndRef={createRef<HTMLDivElement>()}
      />,
    );

    expect(screen.queryByText("user-0")).not.toBeInTheDocument();
    expect(screen.getByText("user-5")).toBeInTheDocument();
    expect(screen.getByText("user-204")).toBeInTheDocument();
  });
});
