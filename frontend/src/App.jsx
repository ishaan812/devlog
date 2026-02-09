import { useState } from 'react'
import { Download, Github, Star, Copy, Check, GitPullRequest, Play } from 'lucide-react'

const ENVIRONMENTS = [
  'ZSH', 'FISH', 'POWERSHELL', 'DOCKER', 'LINUX', 'MACOS', 'WINDOWS WSL', 'BASH',
]

const FEATURES = [
  {
    num: '01 // Automation',
    title: 'Smart Work Logs',
    desc: 'Auto-generated markdown summaries organized by branch, date, and repo — ready for standups, PRs, or performance reviews. Multi-repo, multi-branch, zero effort.',
  },
  {
    num: '02 // Interactive',
    title: 'Console TUI',
    desc: 'Full-screen terminal UI to browse repositories and navigate through your cached worklogs day-by-day. Keyboard shortcuts for quick navigation and formatted markdown viewing.',
  },
  {
    num: '03 // Multi-Repo',
    title: 'Unified Timeline',
    desc: 'Ingest as many repos as you want into a single profile. Frontend, backend, shared libraries — DevLog tracks them all into one unified timeline. See your full picture.',
  },
  {
    num: '04 // Privacy',
    title: 'Local-First AI',
    desc: 'Works completely offline with Ollama. Your code and history stay on your machine. No subscriptions, no telemetry, no cloud dependency. Optionally use cloud providers for pennies per query.',
  },
]

function SkipToContent() {
  return (
    <a
      href="#main-content"
      className="sr-only focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-[200] focus:bg-green focus:text-bg-primary focus:px-4 focus:py-2 focus:rounded focus:font-mono focus:text-sm"
    >
      Skip to main content
    </a>
  )
}

function Navbar() {
  return (
    <nav
      className="fixed top-0 inset-x-0 z-[100] flex items-center justify-between px-5 md:px-12 h-14 bg-[rgba(10,10,10,0.85)] backdrop-blur-[12px] border-b border-border-main"
      aria-label="Main navigation"
    >
      <a
        href="/"
        className="font-mono text-base font-bold text-green no-underline flex items-center gap-1 tracking-[-0.5px]"
        aria-label="DevLog - Home"
      >
        <span className="opacity-50" aria-hidden="true">&gt;_</span> DevLog
      </a>
      <div className="flex items-center gap-4 md:gap-8" role="navigation" aria-label="Site links">
        <a
          href="#features"
          className="hidden md:inline font-mono text-xs text-text-secondary no-underline tracking-[1px] uppercase transition-colors duration-200 hover:text-green"
        >
          /Features
        </a>
        <a
          href="https://github.com/ishaan812/devlog#quick-start"
          target="_blank"
          rel="noopener noreferrer"
          className="hidden md:inline font-mono text-xs text-text-secondary no-underline tracking-[1px] uppercase transition-colors duration-200 hover:text-green"
          aria-label="Quick Start Guide"
        >
          /Docs
        </a>
        <a
          href="https://github.com/ishaan812/devlog"
          target="_blank"
          rel="noopener noreferrer"
          className="flex items-center text-text-secondary transition-colors duration-200 hover:text-green"
          aria-label="DevLog on GitHub"
        >
          <Github size={18} aria-hidden="true" />
        </a>
      </div>
    </nav>
  )
}

function Hero() {
  const [copied, setCopied] = useState(false)
  const [installMethod, setInstallMethod] = useState('npm')

  const commands = {
    npm: 'npm i -g @ishaan812/devlog',
    go: 'go install github.com/ishaan812/devlog/cmd/devlog@latest'
  }

  const handleCopy = () => {
    navigator.clipboard.writeText(commands[installMethod])
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <section
      className="hero-glow pt-[120px] md:pt-[140px] px-5 md:px-6 pb-[60px] md:pb-20 text-center relative overflow-hidden"
      aria-labelledby="hero-title"
    >
      <div
        className="font-display text-5xl md:text-[72px] font-bold text-green tracking-[-2px] mb-4 relative italic [text-shadow:0_0_40px_rgba(0,255,65,0.3)]"
        aria-hidden="true"
      >
        DevLog
      </div>
      <div className="inline-flex items-center gap-2 py-1.5 px-4 bg-[rgba(0,255,65,0.08)] border border-border-green rounded-[20px] text-[10px] tracking-[2px] uppercase text-green mb-10">
        <span className="w-1.5 h-1.5 rounded-full bg-green animate-pulse-dot" aria-hidden="true" />
        <span>System Status: Optimal // v0.0.1</span>
      </div>
      <h1 id="hero-title" className="font-display text-[32px] md:text-[52px] font-bold leading-[1.1] tracking-[-1.5px] mb-6">
        <span className="text-text-primary">Stop Forgetting</span>
        <br />
        <span className="text-green [text-shadow:0_0_30px_rgba(0,255,65,0.3)]">What You Shipped</span>
      </h1>
      <p className="font-mono text-[13px] md:text-sm text-text-secondary leading-[1.8] max-w-[520px] mx-auto mb-10">
        <span className="text-green mr-1" aria-hidden="true">&gt;</span> Local-first AI work logging for developers.
        <br />
        <span className="text-green mr-1" aria-hidden="true">&gt;</span> Multi-repo, multi-branch tracking. One unified timeline.
        <br />
        <span className="text-green mr-1" aria-hidden="true">&gt;</span> Professional markdown work logs from your git history.
      </p>
      <div className="flex flex-col items-center gap-4">
        {/* Install method switcher */}
        <div className="inline-flex items-center gap-1 p-1 bg-bg-secondary border border-border-main rounded-md">
          <button
            onClick={() => setInstallMethod('npm')}
            className={`px-4 py-1.5 rounded font-mono text-xs uppercase tracking-[1px] transition-all duration-200 ${
              installMethod === 'npm'
                ? 'bg-green text-bg-primary font-bold'
                : 'text-text-dim hover:text-text-secondary'
            }`}
            type="button"
            aria-label="Install via npm"
          >
            npm
          </button>
          <button
            onClick={() => setInstallMethod('go')}
            className={`px-4 py-1.5 rounded font-mono text-xs uppercase tracking-[1px] transition-all duration-200 ${
              installMethod === 'go'
                ? 'bg-green text-bg-primary font-bold'
                : 'text-text-dim hover:text-text-secondary'
            }`}
            type="button"
            aria-label="Install via Go"
          >
            go
          </button>
        </div>
        
        <button
          className="group inline-flex items-center gap-3 py-4 px-6 bg-bg-secondary border border-border-main rounded-md text-sm text-text-primary cursor-pointer transition-all duration-200 hover:border-border-green hover:shadow-[0_0_20px_rgba(0,255,65,0.15)] mb-6"
          onClick={handleCopy}
          type="button"
          aria-label={copied ? 'Install command copied to clipboard' : `Copy install command: ${commands[installMethod]}`}
        >
          <span className="text-green" aria-hidden="true">$</span>
          <span className="font-mono">{commands[installMethod]}</span>
          {copied ? (
            <Check size={16} className="text-green" aria-hidden="true" />
          ) : (
            <Copy size={16} className="text-text-dim transition-colors duration-200 group-hover:text-green" aria-hidden="true" />
          )}
        </button>
        <div className="flex gap-4 flex-wrap justify-center">
          <a
            href="https://github.com/ishaan812/devlog"
            target="_blank"
            rel="noopener noreferrer"
            className="btn-clip inline-flex items-center gap-2 py-3.5 px-7 bg-gradient-to-r from-[#febc2e] to-[#ffd35c] text-bg-primary font-mono text-[13px] font-bold no-underline border-none cursor-pointer tracking-[1px] uppercase transition-all duration-200 hover:from-[#ffd35c] hover:to-[#febc2e] hover:shadow-[0_0_30px_rgba(254,188,46,0.5)] hover:scale-105 group"
            aria-label="Star DevLog on GitHub"
          >
            <Star size={16} className="fill-bg-primary group-hover:fill-bg-primary transition-all duration-200" aria-hidden="true" />
            Star on GitHub
          </a>
          <a
            href="https://github.com/ishaan812/devlog#contributing"
            target="_blank"
            rel="noopener noreferrer"
            className="btn-clip inline-flex items-center gap-2 py-3.5 px-7 bg-gradient-to-r from-[#58a6ff] to-[#79c0ff] text-bg-primary font-mono text-[13px] font-bold no-underline border-none cursor-pointer tracking-[1px] uppercase transition-all duration-200 hover:from-[#79c0ff] hover:to-[#58a6ff] hover:shadow-[0_0_30px_rgba(88,166,255,0.5)] hover:scale-105 group"
            aria-label="Contribute to DevLog"
          >
            <GitPullRequest size={16} className="group-hover:rotate-12 transition-all duration-200" aria-hidden="true" />
            Contribute
          </a>
          <a
            href="https://youtu.be/lykiT4GqhR4"
            target="_blank"
            rel="noopener noreferrer"
            className="btn-clip inline-flex items-center gap-2 py-3 px-5 border border-border-main text-text-secondary font-mono text-[12px] font-medium no-underline cursor-pointer tracking-[1px] uppercase transition-all duration-200 hover:bg-white/5 hover:text-text-primary hover:border-text-dim group"
            aria-label="Watch DevLog video on YouTube"
          >
            <Play size={14} className="text-text-dim group-hover:text-text-secondary transition-colors duration-200" aria-hidden="true" />
            Watch Video
          </a>
        </div>
      </div>
    </section>
  )
}

function TerminalDemo() {
  return (
    <section className="px-6 pb-20 flex justify-center" aria-label="Terminal demo">
      <figure
        className="w-full max-w-[720px] bg-bg-terminal border border-border-main rounded-lg overflow-hidden shadow-[0_20px_60px_rgba(0,0,0,0.5)]"
        role="img"
        aria-label="Terminal demonstration showing DevLog generating an activity report from git commits"
      >
        {/* Terminal header */}
        <div className="flex items-center gap-2 py-3 px-4 bg-[#161b22] border-b border-border-main" aria-hidden="true">
          <span className="w-3 h-3 rounded-full bg-[#ff5f57]" />
          <span className="w-3 h-3 rounded-full bg-[#febc2e]" />
          <span className="w-3 h-3 rounded-full bg-[#28c840]" />
          <span className="flex-1 text-center text-xs text-text-dim">root@devlog:~</span>
        </div>
        {/* Terminal body */}
        <div className="py-5 px-6 text-[13px] leading-[1.7]">
        <span className="block mb-0.5">
            <span className="text-green">$ </span>
            <span className="text-text-primary font-medium">devlog onboard</span>
          </span>
          <span className="block mb-0.5">
            <span className="text-green">$ </span>
            <span className="text-text-primary font-medium">devlog ingest ~/projects/frontend</span>
          </span>
          <span className="block mb-0.5">
            <span className="text-[#58a6ff]">→</span>
            <span className="text-text-dim"> Analyzing git history...</span>
          </span>
          <span className="block mb-0.5">
            <span className="text-green">✓</span>
            <span className="text-text-dim"> Processed 147 commits across 3 branches</span>
          </span>
          <span className="block mb-0.5">
            <span className="text-green">✓</span>
            <span className="text-text-dim"> Ingestion complete</span>
          </span>

          <div className="h-3" />

          <span className="block mb-0.5">
            <span className="text-green">$ </span>
            <span className="text-text-primary font-medium">devlog worklog --days 7</span>
          </span>

          <div className="h-3" />

          <span className="block mb-0.5">
            <span className="text-[#febc2e] font-semibold">## Monday, February 10, 2026</span>
          </span>
          <span className="block mb-0.5">
            <span className="text-text-dim">repo: </span>
            <span className="text-[#58a6ff]">frontend</span>
            <span className="text-text-dim"> | Branch: </span>
            <span className="text-green">feature/payments</span>
          </span>

          <div className="h-2" />

          <span className="block mb-0.5">
            <span className="text-text-primary font-medium">### Updates</span>
          </span>
          <span className="block mb-0.5">
            <span className="text-green">•</span>
            <span className="text-text-dim"> Built checkout flow with Stripe integration</span>
          </span>
          <span className="block mb-0.5">
            <span className="text-green">•</span>
            <span className="text-text-dim"> Added client-side form validation</span>
          </span>

          <div className="h-2" />

          <span className="block mb-0.5">
            <span className="text-text-primary font-medium">### Commits</span>
          </span>
          <span className="block mb-0.5">
            <span className="text-[#febc2e]">16:45</span>
            <span className="text-text-dim"> </span>
            <span className="text-[#58a6ff]">f8a9b0c</span>
            <span className="text-text-dim"> Add Stripe checkout component </span>
            <span className="text-green">(+320/-15)</span>
          </span>
          <span className="block mb-0.5">
            <span className="text-[#febc2e]">14:20</span>
            <span className="text-text-dim"> </span>
            <span className="text-[#58a6ff]">c3d4e5f</span>
            <span className="text-text-dim"> Add card validation helpers </span>
            <span className="text-green">(+95/-10)</span>
          </span>

          <div className="h-3" />

          <span className="block mb-0.5">
            <span className="text-green">$ </span>
            <span className="text-text-primary font-medium">devlog console </span>
            <span className="inline-block w-2 h-4 bg-green animate-blink align-text-bottom" aria-hidden="true" />
          </span>
          <span className="block mb-0.5">
            <span className="text-[#58a6ff]">→</span>
            <span className="text-text-dim"> Launching interactive TUI...</span>
          </span>
        </div>
      </figure>
    </section>
  )
}

function Features() {
  return (
    <section className="py-[100px] px-6 max-w-[1100px] mx-auto" id="features" aria-labelledby="features-title">
      <header className="mb-16">
        <h2 id="features-title" className="font-display text-[28px] md:text-4xl font-bold leading-[1.2] tracking-[-1px]">
          Why Developers
          <br />
          <span className="text-green">Love DevLog</span>
        </h2>
        <p className="mt-3 text-sm text-text-secondary max-w-[540px]">
          You context-switch constantly. Your standups are painful. You work across multiple repos.
          DevLog tracks all of it — across repos and branches — so you don't have to.
        </p>
      </header>
      <div className="grid grid-cols-1 md:grid-cols-2 gap-6" role="list">
        {FEATURES.map((f, i) => (
          <article
            className="p-8 bg-bg-card border border-border-main rounded-[4px] transition-all duration-300 relative hover:border-border-green hover:shadow-[0_0_30px_rgba(0,255,65,0.15)]"
            key={i}
            role="listitem"
          >
            <div className="text-[11px] text-text-dim tracking-[2px] uppercase mb-4" aria-hidden="true">{f.num}</div>
            <h3 className="font-display text-xl font-semibold text-text-primary mb-3">{f.title}</h3>
            <p className="text-[13px] text-text-secondary leading-[1.7]">{f.desc}</p>
          </article>
        ))}
      </div>
    </section>
  )
}

function Marquee() {
  const items = [...ENVIRONMENTS, ...ENVIRONMENTS, ...ENVIRONMENTS]
  return (
    <section className="py-[60px] overflow-hidden border-y border-border-main" aria-label="Compatible environments">
      <div className="text-center text-[10px] text-green tracking-[3px] uppercase mb-8" aria-hidden="true">
        &lt; Compatible Environments /&gt;
      </div>
      <div className="flex w-max animate-marquee hover:[animation-play-state:paused]" aria-hidden="true">
        {items.map((env, i) => (
          <span
            className="shrink-0 px-10 text-sm font-medium text-text-dim tracking-[3px] uppercase whitespace-nowrap transition-colors duration-200 hover:text-green"
            key={i}
          >
            {env}
          </span>
        ))}
      </div>
      {/* Accessible list of environments for screen readers */}
      <div className="sr-only">
        <p>Compatible with: {ENVIRONMENTS.join(', ')}</p>
      </div>
    </section>
  )
}

function CTA() {
  const [copied, setCopied] = useState(false)
  const [installMethod, setInstallMethod] = useState('npm')

  const commands = {
    npm: 'npm i -g @ishaan812/devlog',
    go: 'go install github.com/ishaan812/devlog/cmd/devlog@latest'
  }

  const handleCopy = () => {
    navigator.clipboard.writeText(commands[installMethod])
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <section className="py-[120px] px-6 text-center" aria-labelledby="cta-title" id="pricing">
      <span className="text-green text-[40px] font-bold font-display" aria-hidden="true">&gt; </span>
      <h2 id="cta-title" className="font-display text-[32px] md:text-5xl font-bold tracking-[-1.5px] mb-4">
        Ready to commit?
      </h2>
      <p className="text-sm text-text-secondary mb-10 max-w-[480px] mx-auto">
        Professional markdown work logs, generated from your actual commits.<br />
        Multi-repo, multi-branch, zero effort.
      </p>
      
      <div className="flex flex-col items-center gap-4">
        {/* Install method switcher */}
        <div className="inline-flex items-center gap-1 p-1 bg-bg-secondary border border-border-main rounded-md">
          <button
            onClick={() => setInstallMethod('npm')}
            className={`px-4 py-1.5 rounded font-mono text-xs uppercase tracking-[1px] transition-all duration-200 ${
              installMethod === 'npm'
                ? 'bg-green text-bg-primary font-bold'
                : 'text-text-dim hover:text-text-secondary'
            }`}
            type="button"
            aria-label="Install via npm"
          >
            npm
          </button>
          <button
            onClick={() => setInstallMethod('go')}
            className={`px-4 py-1.5 rounded font-mono text-xs uppercase tracking-[1px] transition-all duration-200 ${
              installMethod === 'go'
                ? 'bg-green text-bg-primary font-bold'
                : 'text-text-dim hover:text-text-secondary'
            }`}
            type="button"
            aria-label="Install via Go"
          >
            go
          </button>
        </div>
        
        <button
          className="group inline-flex items-center gap-3 py-4 px-6 bg-bg-secondary border border-border-main rounded-md text-sm text-text-primary cursor-pointer transition-all duration-200 hover:border-border-green hover:shadow-[0_0_20px_rgba(0,255,65,0.15)]"
          onClick={handleCopy}
          type="button"
          aria-label={copied ? 'Install command copied to clipboard' : `Copy install command: ${commands[installMethod]}`}
        >
          <span className="text-green" aria-hidden="true">$</span>
          <span className="font-mono">{commands[installMethod]}</span>
          {copied ? (
            <Check size={16} className="text-green" aria-hidden="true" />
          ) : (
            <Copy size={16} className="text-text-dim transition-colors duration-200 group-hover:text-green" aria-hidden="true" />
          )}
        </button>
      </div>
      
      <span className="block mt-6 text-xs text-text-dim">Free &amp; Open Source. MIT License. Local-first.</span>
    </section>
  )
}

function Footer() {
  return (
    <footer className="py-6 px-5 md:px-12 flex flex-col md:flex-row items-center justify-between gap-4 md:gap-0 text-center md:text-left border-t border-border-main text-[11px] text-text-dim">
      <div className="flex flex-col gap-1 items-center md:items-start">
        <span className="font-mono font-bold text-green text-[13px]">
          <span className="opacity-50" aria-hidden="true">&gt;_ </span>DevLog
        </span>
        <a 
          href="https://devlog.ishaan812.com" 
          className="text-text-dim no-underline text-[10px] transition-colors duration-200 hover:text-green"
        >
          devlog.ishaan812.com
        </a>
      </div>
      <nav className="flex gap-6" aria-label="Footer navigation">
        <a href="https://www.npmjs.com/package/@ishaan812/devlog" target="_blank" rel="noopener noreferrer" className="text-text-dim no-underline uppercase tracking-[1px] text-[11px] transition-colors duration-200 hover:text-green">NPM</a>
        <a href="https://github.com/ishaan812/devlog#quick-start" target="_blank" rel="noopener noreferrer" className="text-text-dim no-underline uppercase tracking-[1px] text-[11px] transition-colors duration-200 hover:text-green">Docs</a>
        <a href="https://github.com/ishaan812/devlog" target="_blank" rel="noopener noreferrer" className="text-text-dim no-underline uppercase tracking-[1px] text-[11px] transition-colors duration-200 hover:text-green">GitHub</a>
        <a href="https://github.com/ishaan812/devlog/blob/master/LICENSE" target="_blank" rel="noopener noreferrer" className="text-text-dim no-underline uppercase tracking-[1px] text-[11px] transition-colors duration-200 hover:text-green">MIT License</a>
      </nav>
    </footer>
  )
}

function App() {
  return (
    <>
      <SkipToContent />
      <Navbar />
      <main id="main-content">
        <Hero />
        <TerminalDemo />
        <Features />
        <Marquee />
        <CTA />
      </main>
      <Footer />
    </>
  )
}

export default App
