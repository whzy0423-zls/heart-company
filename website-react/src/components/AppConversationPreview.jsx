export default function AppConversationPreview() {
  return (
    <div className="app-conversation">
      <header className="app-conversation__header">
        <span className="app-conversation__back" aria-hidden="true">‹</span>
        <img src="/assets/logo.svg" alt="" width="34" height="34" />
        <div className="app-conversation__identity">
          <strong>芯之力 AI</strong>
          <span><i aria-hidden="true" />正在陪伴</span>
        </div>
        <span className="app-conversation__sample">匿名示例对话</span>
      </header>

      <div className="app-conversation__body">
        <time className="app-conversation__date" dateTime="21:08">今天 21:08</time>

        <div className="app-conversation__topic">
          <span>关系 · 边界感</span>
          <strong>把感受说清楚</strong>
        </div>

        <div className="app-conversation__message app-conversation__message--user">
          <div className="app-conversation__bubble">
            我明明很累，却总怕拒绝别人会让关系变差。
          </div>
          <time dateTime="21:08">21:08</time>
        </div>

        <div className="app-conversation__message app-conversation__message--assistant">
          <img src="/assets/logo.svg" alt="" width="30" height="30" />
          <div>
            <div className="app-conversation__bubble">
              你担心的也许不是拒绝本身，而是拒绝之后失去认可。
              <strong>照顾关系，不等于答应所有请求。</strong>
            </div>
            <div className="app-conversation__practice">
              <span>今天可以试试</span>
              <p>“我现在精力有限，这件事明天下午再回复你，可以吗？”</p>
            </div>
            <time dateTime="21:09">21:09 · 已为你整理</time>
          </div>
        </div>

        <div className="app-conversation__message app-conversation__message--user app-conversation__message--question">
          <div className="app-conversation__bubble">如果对方不高兴呢？</div>
          <time dateTime="21:10">21:10</time>
        </div>

        <div className="app-conversation__message app-conversation__message--assistant app-conversation__message--answer">
          <img src="/assets/logo.svg" alt="" width="30" height="30" />
          <div>
            <div className="app-conversation__bubble">
              对方失望，不代表你做错了。边界不是推开别人，而是在关系里保持真实。
            </div>
            <time dateTime="21:10">21:10</time>
          </div>
        </div>

        <div className="app-conversation__follow-up">
          <span aria-hidden="true" />
          <p>你不需要一次做到完美，先从一句诚实的表达开始。</p>
        </div>
      </div>

      <div className="app-conversation__composer" aria-label="对话输入预览">
        <span className="app-conversation__voice" aria-hidden="true">
          <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor"
               strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
            <rect x="9" y="3" width="6" height="11" rx="3" />
            <path d="M6 11a6 6 0 0 0 12 0M12 17v4M9 21h6" />
          </svg>
        </span>
        <span className="app-conversation__placeholder">继续聊聊此刻的感受…</span>
        <span className="app-conversation__send" aria-hidden="true">
          <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor"
               strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="m5 12 14-7-4 14-3-6-7-1ZM12 13l3-3" />
          </svg>
        </span>
      </div>
    </div>
  )
}
