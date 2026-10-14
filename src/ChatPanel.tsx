import { useEffect, useRef, useState, type FormEvent, type KeyboardEvent } from 'react';
import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { formatPeriod } from './energy';
import { streamChat, type ChatMessage, type ChatView } from './services/chatService';

const maxHistoryMessages = 20;
const maxQuestionLength = 2000;
const maxAssistantLength = 8000;

const toolLabels: Record<string, string> = {
  get_monthly_data: 'Reading monthly generation data',
  get_annual_summary: 'Totaling annual generation',
  get_eia_forecast: 'Checking the EIA forecast',
  get_milestones: 'Looking up milestones',
};

interface ChatPanelProps {
  view: ChatView;
}

export function ChatPanel({ view }: ChatPanelProps) {
  const [open, setOpen] = useState(false);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [draft, setDraft] = useState('');
  const [streaming, setStreaming] = useState(false);
  const [activity, setActivity] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const controllerRef = useRef<AbortController | null>(null);
  const messagesRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    const list = messagesRef.current;
    if (list) {
      list.scrollTop = list.scrollHeight;
    }
  }, [messages, activity, error]);

  useEffect(() => {
    if (open) {
      inputRef.current?.focus();
    }
  }, [open]);

  useEffect(() => () => controllerRef.current?.abort(), []);

  const suggestions = [
    `What stands out in ${formatPeriod(view.period)}?`,
    'How fast is solar growing, and what does EIA expect next?',
    'Which sources are losing share, and to what?',
    'What do these trends mean for energy investors?',
  ];

  async function ask(question: string) {
    const text = question.trim();
    if (!text || streaming) {
      return;
    }

    const history: ChatMessage[] = [
      ...messages
        .filter((message) => message.content.trim() !== '')
        .map((message) => ({ ...message, content: message.content.slice(0, maxAssistantLength) })),
      { role: 'user', content: text },
    ];
    setMessages([...history, { role: 'assistant', content: '' }]);
    setDraft('');
    setError(null);
    setStreaming(true);

    const controller = new AbortController();
    controllerRef.current = controller;
    try {
      await streamChat(
        history.slice(-maxHistoryMessages),
        view,
        (event) => {
          switch (event.type) {
            case 'text':
              setActivity(null);
              setMessages((current) => {
                const last = current[current.length - 1];
                if (last?.role !== 'assistant') {
                  return current;
                }
                return [...current.slice(0, -1), { ...last, content: last.content + event.delta }];
              });
              break;
            case 'tool':
              setActivity(toolLabels[event.tool] ?? 'Looking up data');
              break;
            case 'error':
              setError(event.message);
              break;
          }
        },
        controller.signal
      );
    } catch (err) {
      if (!controller.signal.aborted) {
        setError(err instanceof Error ? err.message : 'The assistant is unavailable.');
      }
    } finally {
      setStreaming(false);
      setActivity(null);
      setMessages((current) => current.filter((message) => message.content.trim() !== ''));
      controllerRef.current = null;
    }
  }

  function submit(event: FormEvent) {
    event.preventDefault();
    ask(draft);
  }

  function handleKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault();
      ask(draft);
    }
  }

  function clear() {
    controllerRef.current?.abort();
    setMessages([]);
    setError(null);
  }

  if (!open) {
    return (
      <button type="button" className="chat-launcher" onClick={() => setOpen(true)}>
        <span aria-hidden="true">✦</span> Ask AI
      </button>
    );
  }

  const lastMessage = messages[messages.length - 1];
  const waiting = streaming && (lastMessage?.role !== 'assistant' || lastMessage.content === '');

  return (
    <aside className="chat-panel" aria-label="AI assistant">
      <header className="chat-header">
        <div>
          <h3>Ask about the data</h3>
          <p>Viewing {formatPeriod(view.period)}{view.forecast ? ' · EIA forecast' : ''}</p>
        </div>
        <div className="chat-header-actions">
          {messages.length > 0 && (
            <button type="button" className="chat-text-button" onClick={clear}>
              Clear
            </button>
          )}
          <button type="button" className="chat-close" onClick={() => setOpen(false)} aria-label="Close assistant">
            ×
          </button>
        </div>
      </header>

      <div className="chat-messages" ref={messagesRef}>
        {messages.length === 0 && (
          <div className="chat-empty">
            <p>Answers are grounded in the EIA generation data and forecast behind this dashboard.</p>
            <div className="chat-suggestions">
              {suggestions.map((suggestion) => (
                <button key={suggestion} type="button" onClick={() => ask(suggestion)}>
                  {suggestion}
                </button>
              ))}
            </div>
          </div>
        )}

        {messages.map((message, index) =>
          message.role === 'user' ? (
            <div key={index} className="chat-message user">
              {message.content}
            </div>
          ) : (
            message.content && (
              <div key={index} className="chat-message assistant">
                <Markdown
                  remarkPlugins={[remarkGfm]}
                  components={{ a: (props) => <a {...props} target="_blank" rel="noreferrer" /> }}
                >
                  {message.content}
                </Markdown>
              </div>
            )
          )
        )}

        {waiting && (
          <div className="chat-activity">
            <span className="chat-dots" aria-hidden="true">
              <span />
              <span />
              <span />
            </span>
            {activity ?? 'Thinking'}…
          </div>
        )}
        {error && <div className="chat-error">{error}</div>}
      </div>

      <form className="chat-input" onSubmit={submit}>
        <textarea
          ref={inputRef}
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Ask about trends, forecasts, or what they could mean…"
          rows={2}
          maxLength={maxQuestionLength}
        />
        {streaming ? (
          <button type="button" className="chat-send" onClick={() => controllerRef.current?.abort()}>
            Stop
          </button>
        ) : (
          <button type="submit" className="chat-send" disabled={!draft.trim()}>
            Send
          </button>
        )}
      </form>
      <p className="chat-disclaimer">AI-generated from EIA data. Educational only, not financial advice.</p>
    </aside>
  );
}
