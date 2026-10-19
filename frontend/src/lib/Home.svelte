<script lang="ts">
  import Icon from './Icon.svelte'
  import type { DocInfo } from './types'

  export let recent: DocInfo[] = []
  export let opening = false
  export let error = ''
  export let onOpen: () => void = () => {}
  export let onOpenRecent: (doc: DocInfo) => void = () => {}
  export let onRemoveRecent: (doc: DocInfo) => void = () => {}

  $: continueDoc = recent.find((item) => item.exists) || null

  function formatRelative(timestamp: number) {
    if (!timestamp) return ''
    const diff = Date.now() - timestamp
    if (diff < 60_000) return '刚刚'
    if (diff < 3_600_000) return `${Math.floor(diff / 60_000)} 分钟前`
    if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)} 小时前`
    if (diff < 7 * 86_400_000) return `${Math.floor(diff / 86_400_000)} 天前`
    return new Date(timestamp).toLocaleDateString('zh-CN', {
      month: 'numeric',
      day: 'numeric'
    })
  }

  function progressPercent(doc: DocInfo) {
    if (!doc.pageCount) return 0
    return Math.min(100, Math.max(0, (doc.lastPage / doc.pageCount) * 100))
  }
</script>

<div class="home">
  <header class="home-header">
    <div class="brand">
      <div class="brand-mark">
        <span class="brand-page"></span>
        <span class="brand-ink"></span>
      </div>
      <div class="brand-text">
        <strong>SimpleReader</strong>
        <span>本地 PDF 阅读器</span>
      </div>
    </div>
    <div class="local-note">
      <span class="local-dot"></span>
      本地运行，不上传文件
    </div>
  </header>

  <main class="home-main">
    <section class="hero">
      <div class="eyebrow">READ · MARK · RETURN</div>
      <h1>打开即续读，<br /><em>笔落即批注。</em></h1>
      <p class="hero-copy">
        不用建书库，不用手动加书签。关闭再打开，还是刚才那一页。
      </p>

      <div class="hero-actions">
        {#if continueDoc}
          <button class="primary-button" on:click={() => onOpenRecent(continueDoc)}>
            <span class="button-icon"><Icon name="book" size={19} /></span>
            <span class="button-copy">
              <span>继续阅读</span>
              <small>{continueDoc.name}</small>
            </span>
            <span class="button-arrow"><Icon name="right" size={17} /></span>
          </button>
        {/if}
        <button class="secondary-button" on:click={onOpen} disabled={opening}>
          {#if opening}
            <span class="mini-spinner"></span>
            正在打开…
          {:else}
            <Icon name="open" size={18} />
            打开 PDF
          {/if}
        </button>
      </div>

      <div class="drop-hint">
        <span class="drop-line"></span>
        也可以把 PDF 直接拖进这个窗口
        <span class="drop-line"></span>
      </div>

      {#if error}
        <div class="home-error">{error}</div>
      {/if}
    </section>

    <section class="recent-section">
      <div class="section-head">
        <h2>最近阅读</h2>
        <span>{recent.length ? `${recent.length} 份文档` : '还没有打开过文档'}</span>
      </div>

      {#if recent.length === 0}
        <div class="empty-state">
          <div class="empty-icon"><Icon name="file" size={30} /></div>
          <div>
            <strong>从一份 PDF 开始</strong>
            <p>打开后，阅读位置和鼠标批注会自动保存在本机。</p>
          </div>
        </div>
      {:else}
        <div class="doc-grid">
          {#each recent as item (item.id)}
            <article class="doc-card" class:missing={!item.exists}>
              <button
                class="doc-open"
                on:click={() => onOpenRecent(item)}
                disabled={!item.exists}
                title={item.path}
              >
                <div class="doc-icon"><Icon name="file" size={21} /></div>
                <div class="doc-info">
                  <div class="doc-name">{item.name}</div>
                  <div class="doc-sub">
                    {#if item.exists}
                      {#if item.pageCount}
                        第 {item.lastPage} / {item.pageCount} 页
                      {:else}
                        未打开过
                      {/if}
                      {#if item.updatedAt}
                        <span class="dot">·</span>{formatRelative(item.updatedAt)}
                      {/if}
                    {:else}
                      文件已移动或删除
                    {/if}
                  </div>
                  {#if item.exists && item.pageCount}
                    <div class="doc-progress">
                      <div style:width={`${progressPercent(item)}%`}></div>
                    </div>
                  {/if}
                </div>
                {#if item.exists}
                  <span class="doc-arrow"><Icon name="right" size={16} /></span>
                {/if}
              </button>
              <button
                class="remove-button"
                title="从最近列表移除"
                aria-label="从最近列表移除"
                on:click={() => onRemoveRecent(item)}
              >
                <Icon name="trash" size={15} />
              </button>
            </article>
          {/each}
        </div>
      {/if}
    </section>

    <section class="principles">
      <div class="principle">
        <span class="principle-number">01</span>
        <div><strong>笔落即写</strong><p>按 P 进入批注，鼠标直接画，滚轮照常翻页。</p></div>
      </div>
      <div class="principle">
        <span class="principle-number">02</span>
        <div><strong>退出即存</strong><p>阅读位置和笔迹自动保存，最多丢最后一笔。</p></div>
      </div>
      <div class="principle">
        <span class="principle-number">03</span>
        <div><strong>打开即续</strong><p>按文档指纹识别，改名或移动后仍能找回位置。</p></div>
      </div>
    </section>
  </main>

  <footer class="home-footer">
    <span>PDF 与批注均保存在本机</span>
    <span>SimpleReader</span>
  </footer>
</div>

<style>
  .home {
    min-height: 100vh;
    display: flex;
    flex-direction: column;
    background:
      radial-gradient(circle at 12% 4%, rgba(200, 85, 61, 0.07), transparent 28%),
      radial-gradient(circle at 88% 18%, rgba(47, 107, 255, 0.05), transparent 24%),
      #f6f4ef;
    color: #20242b;
    overflow-y: auto;
  }

  .home-header {
    width: min(1060px, calc(100% - 80px));
    margin: 0 auto;
    padding: 26px 0 18px;
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .brand {
    display: flex;
    align-items: center;
    gap: 11px;
  }

  .brand-mark {
    position: relative;
    width: 34px;
    height: 38px;
    border-radius: 7px;
    background: #20242b;
    box-shadow: 0 5px 14px rgba(32, 36, 43, 0.18);
  }

  .brand-page {
    position: absolute;
    left: 10px;
    top: 8px;
    width: 14px;
    height: 20px;
    border-radius: 2px;
    background: #f6f4ef;
  }

  .brand-ink {
    position: absolute;
    right: 6px;
    bottom: 7px;
    width: 12px;
    height: 3px;
    border-radius: 2px;
    background: #c8553d;
    transform: rotate(-38deg);
  }

  .brand-text {
    display: flex;
    flex-direction: column;
    line-height: 1.2;
  }

  .brand-text strong {
    font-size: 14px;
    letter-spacing: -0.01em;
  }

  .brand-text span {
    margin-top: 2px;
    font-size: 10px;
    color: #87827a;
    letter-spacing: 0.05em;
  }

  .local-note {
    display: flex;
    align-items: center;
    gap: 7px;
    padding: 7px 12px;
    border: 1px solid rgba(32, 36, 43, 0.09);
    border-radius: 999px;
    background: rgba(255, 255, 255, 0.55);
    color: #77736b;
    font-size: 10px;
    letter-spacing: 0.02em;
  }

  .local-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: #238b63;
    box-shadow: 0 0 0 3px rgba(35, 139, 99, 0.12);
  }

  .home-main {
    width: min(1060px, calc(100% - 80px));
    margin: 0 auto;
    flex: 1;
  }

  .hero {
    padding: 46px 0 38px;
    max-width: 720px;
  }

  .eyebrow {
    margin-bottom: 17px;
    color: #c8553d;
    font-size: 10px;
    font-weight: 750;
    letter-spacing: 0.18em;
  }

  h1 {
    margin: 0;
    font-size: clamp(36px, 5vw, 54px);
    line-height: 1.1;
    letter-spacing: -0.045em;
    font-weight: 730;
  }

  h1 em {
    font-style: normal;
    color: #c8553d;
  }

  .hero-copy {
    max-width: 480px;
    margin: 20px 0 0;
    color: #6f6b63;
    font-size: 14px;
    line-height: 1.75;
  }

  .hero-actions {
    display: flex;
    align-items: stretch;
    gap: 10px;
    margin-top: 28px;
    flex-wrap: wrap;
  }

  .primary-button,
  .secondary-button {
    min-height: 50px;
    display: inline-flex;
    align-items: center;
    gap: 10px;
    padding: 0 17px;
    border-radius: 12px;
    font: inherit;
    font-size: 12px;
    font-weight: 650;
    cursor: pointer;
    transition: transform 140ms ease, box-shadow 140ms ease, background 140ms ease;
  }

  .primary-button {
    border: 1px solid #20242b;
    background: #20242b;
    color: #fff;
    box-shadow: 0 10px 26px rgba(32, 36, 43, 0.16);
  }

  .primary-button:hover {
    transform: translateY(-1px);
    box-shadow: 0 14px 32px rgba(32, 36, 43, 0.2);
  }

  .button-icon {
    width: 32px;
    height: 32px;
    display: grid;
    place-items: center;
    border-radius: 8px;
    background: rgba(255, 255, 255, 0.11);
    color: #f0c96a;
  }

  .button-copy {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 2px;
    max-width: 250px;
  }

  .button-copy small {
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: rgba(255, 255, 255, 0.58);
    font-size: 10px;
    font-weight: 500;
  }

  .button-arrow {
    margin-left: 7px;
    color: rgba(255, 255, 255, 0.55);
  }

  .secondary-button {
    border: 1px solid rgba(32, 36, 43, 0.14);
    background: rgba(255, 255, 255, 0.7);
    color: #34383f;
  }

  .secondary-button:hover {
    background: #fff;
    border-color: rgba(32, 36, 43, 0.28);
  }

  .secondary-button:disabled {
    opacity: 0.6;
    cursor: default;
  }

  .mini-spinner {
    width: 15px;
    height: 15px;
    border: 2px solid rgba(32, 36, 43, 0.15);
    border-top-color: #c8553d;
    border-radius: 50%;
    animation: spin 700ms linear infinite;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  .drop-hint {
    display: flex;
    align-items: center;
    gap: 12px;
    width: min(430px, 100%);
    margin-top: 22px;
    color: #9a958c;
    font-size: 10px;
    letter-spacing: 0.02em;
  }

  .drop-line {
    height: 1px;
    flex: 1;
    background: rgba(32, 36, 43, 0.1);
  }

  .home-error {
    display: inline-block;
    margin-top: 16px;
    padding: 9px 12px;
    border-radius: 8px;
    background: #fbe9e5;
    color: #a83d2e;
    font-size: 11px;
    line-height: 1.5;
  }

  .recent-section {
    padding: 8px 0 34px;
  }

  .section-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    padding-bottom: 13px;
    border-bottom: 1px solid rgba(32, 36, 43, 0.1);
  }

  .section-head h2 {
    margin: 0;
    font-size: 15px;
    font-weight: 680;
    letter-spacing: -0.01em;
  }

  .section-head span {
    color: #969188;
    font-size: 10px;
  }

  .doc-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 9px;
    margin-top: 14px;
  }

  .doc-card {
    position: relative;
    border: 1px solid rgba(32, 36, 43, 0.09);
    border-radius: 13px;
    background: rgba(255, 255, 255, 0.62);
    transition: background 140ms ease, border-color 140ms ease, transform 140ms ease;
  }

  .doc-card:hover {
    background: #fff;
    border-color: rgba(32, 36, 43, 0.18);
    transform: translateY(-1px);
  }

  .doc-card.missing {
    opacity: 0.62;
  }

  .doc-open {
    width: 100%;
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 13px 42px 13px 13px;
    border: 0;
    background: transparent;
    color: inherit;
    text-align: left;
    cursor: pointer;
  }

  .doc-open:disabled {
    cursor: default;
  }

  .doc-icon {
    width: 38px;
    height: 44px;
    flex: 0 0 38px;
    display: grid;
    place-items: center;
    border-radius: 7px;
    background: #efe9df;
    color: #8a8378;
  }

  .doc-info {
    min-width: 0;
    flex: 1;
  }

  .doc-name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 12px;
    font-weight: 650;
    letter-spacing: -0.005em;
  }

  .doc-sub {
    margin-top: 4px;
    color: #8d887f;
    font-size: 10px;
  }

  .dot {
    margin: 0 4px;
    color: #b6b1a8;
  }

  .doc-progress {
    height: 2px;
    margin-top: 8px;
    border-radius: 2px;
    background: rgba(32, 36, 43, 0.07);
    overflow: hidden;
  }

  .doc-progress div {
    height: 100%;
    border-radius: 2px;
    background: #c8553d;
  }

  .doc-arrow {
    position: absolute;
    right: 15px;
    top: 50%;
    transform: translateY(-50%);
    color: #b5afa5;
  }

  .remove-button {
    position: absolute;
    right: 8px;
    top: 8px;
    width: 24px;
    height: 24px;
    display: grid;
    place-items: center;
    border: 0;
    border-radius: 7px;
    background: transparent;
    color: #aaa49a;
    cursor: pointer;
    opacity: 0;
    transition: opacity 120ms ease, background 120ms ease, color 120ms ease;
  }

  .doc-card:hover .remove-button {
    opacity: 1;
  }

  .remove-button:hover {
    background: #fbe9e5;
    color: #b23a2a;
  }

  .empty-state {
    display: flex;
    align-items: center;
    gap: 16px;
    margin-top: 14px;
    padding: 25px;
    border: 1px dashed rgba(32, 36, 43, 0.16);
    border-radius: 13px;
    color: #858077;
  }

  .empty-icon {
    width: 52px;
    height: 58px;
    display: grid;
    place-items: center;
    border-radius: 9px;
    background: #efe9df;
    color: #989084;
  }

  .empty-state strong {
    display: block;
    color: #56524b;
    font-size: 12px;
  }

  .empty-state p {
    margin: 4px 0 0;
    font-size: 10px;
    line-height: 1.6;
  }

  .principles {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 22px;
    padding: 26px 0 34px;
    border-top: 1px solid rgba(32, 36, 43, 0.08);
  }

  .principle {
    display: flex;
    gap: 11px;
  }

  .principle-number {
    color: #c8553d;
    font-size: 9px;
    font-weight: 750;
    letter-spacing: 0.08em;
    padding-top: 2px;
  }

  .principle strong {
    display: block;
    font-size: 11px;
    font-weight: 650;
  }

  .principle p {
    margin: 4px 0 0;
    color: #8b867d;
    font-size: 10px;
    line-height: 1.6;
  }

  .home-footer {
    width: min(1060px, calc(100% - 80px));
    margin: 0 auto;
    padding: 16px 0 22px;
    display: flex;
    justify-content: space-between;
    border-top: 1px solid rgba(32, 36, 43, 0.08);
    color: #a49e94;
    font-size: 9px;
    letter-spacing: 0.04em;
  }

  @media (max-width: 820px) {
    .home-header,
    .home-main,
    .home-footer {
      width: calc(100% - 40px);
    }

    .doc-grid,
    .principles {
      grid-template-columns: 1fr;
    }

    .local-note {
      display: none;
    }

    .hero {
      padding-top: 30px;
    }
  }
</style>
