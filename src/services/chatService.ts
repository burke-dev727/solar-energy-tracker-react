export interface ChatMessage {
  role: 'user' | 'assistant';
  content: string;
}

/** The dashboard month the user is looking at when they ask. */
export interface ChatView {
  period: string;
  forecast: boolean;
}

export type ChatEvent =
  | { type: 'text'; delta: string }
  | { type: 'tool'; tool: string }
  | { type: 'error'; message: string }
  | { type: 'done' };

export async function getChatEnabled(signal?: AbortSignal): Promise<boolean> {
  const response = await fetch('/api/chat/status', { signal });
  if (!response.ok) {
    return false;
  }
  const status: { enabled: boolean } = await response.json();
  return status.enabled;
}

/** Sends the conversation and calls onEvent for each event the server streams back. */
export async function streamChat(
  messages: ChatMessage[],
  view: ChatView,
  onEvent: (event: ChatEvent) => void,
  signal?: AbortSignal
): Promise<void> {
  const response = await fetch('/api/chat', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ messages, view }),
    signal,
  });
  if (!response.ok || !response.body) {
    const message = (await response.text()).trim();
    throw new Error(message || `Chat request failed: ${response.status}`);
  }

  const reader = response.body.pipeThrough(new TextDecoderStream()).getReader();
  let buffer = '';
  for (;;) {
    const { value, done } = await reader.read();
    if (done) {
      return;
    }
    buffer += value;

    let boundary;
    while ((boundary = buffer.indexOf('\n\n')) >= 0) {
      const data = buffer
        .slice(0, boundary)
        .split('\n')
        .filter((line) => line.startsWith('data: '))
        .map((line) => line.slice('data: '.length))
        .join('\n');
      buffer = buffer.slice(boundary + 2);
      if (data) {
        onEvent(JSON.parse(data));
      }
    }
  }
}
