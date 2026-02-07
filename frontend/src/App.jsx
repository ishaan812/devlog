import { useState } from 'react'
import { Download, Github, Star, Copy, Check } from 'lucide-react'

const ENVIRONMENTS = [
  'ZSH', 'FISH', 'POWERSHELL', 'DOCKER', 'LINUX', 'MACOS', 'WINDOWS WSL', 'BASH',
]

const FEATURES = [
  {
    num: '01 // Automation',
    title: 'Zero-Touch Changelogs',
    desc: 'Parsing commit messages using conventional commits standard to auto-generate semantic version updates, changelogs, and manual documentation overhead by 100%.',
  },
  {
    num: '02 // Integration',
    title: 'Stack Agnostic',
    desc: "Works with git. That's it. Whether you use Node, Rust, Go, or Python — pull diffs directly from the CLI. Compatible with GitHub, GitLab, and Bitbucket.",
  },
  {
    num: '03 // Intelligence',
    title: 'Context Aware NLP',
    desc: 'AI that understands code — summarizes complex diffs into human-readable bullet points. Detects your architecture, naming and scopes changes across your local codebase.',
  },
  {
    num: '04 // Analytics',
    title: 'Velocity Metrics',
    desc: 'Track commit trends, weekly output metrics from codebase changelog generation to new feature burn. Get actionable analytics for your next sprint planning or sprint planning.',
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
          href="https://github.com/ishaan812/devlog"
          target="_blank"
          rel="noopener noreferrer"
          className="hidden md:inline font-mono text-xs text-text-secondary no-underline tracking-[1px] uppercase transition-colors duration-200 hover:text-green"
          aria-label="Documentation on GitHub"
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
        <span>System Status: Optimal // v0.0.4</span>
      </div>
      <h1 id="hero-title" className="font-display text-[32px] md:text-[52px] font-bold leading-[1.1] tracking-[-1.5px] mb-6">
        <span className="text-text-primary">Stop Forgetting</span>
        <br />
        <span className="text-green [text-shadow:0_0_30px_rgba(0,255,65,0.3)]">What You Shipped</span>
      </h1>
      <p className="font-mono text-[13px] md:text-sm text-text-secondary leading-[1.8] max-w-[500px] mx-auto mb-10">
        <span className="text-green mr-1" aria-hidden="true">&gt;</span> An AI-powered CLI activity tracker for developers.
        <br />
        <span className="text-green mr-1" aria-hidden="true">&gt;</span> Turn git commits into changelogs &amp; standups instantly.
        <br />
        <span className="text-green mr-1" aria-hidden="true">&gt;</span> Local LLM compatible &amp; Open Source MIT License.
      </p>
      <div className="flex gap-4 justify-center flex-wrap" role="group" aria-label="Get started">
        <a
          href="https://www.npmjs.com/package/@devlog/cli"
          target="_blank"
          rel="noopener noreferrer"
          className="btn-clip inline-flex items-center gap-2 py-3.5 px-7 bg-green text-bg-primary font-mono text-[13px] font-semibold no-underline border-none cursor-pointer tracking-[1px] uppercase transition-all duration-200 hover:bg-[#33ff66] hover:shadow-[0_0_30px_rgba(0,255,65,0.3)]"
          aria-label="Install DevLog via NPM"
        >
          <Download size={16} aria-hidden="true" />
          Install via NPM
        </a>
        <a
          href="https://github.com/ishaan812/devlog"
          target="_blank"
          rel="noopener noreferrer"
          className="btn-clip inline-flex items-center gap-2 py-3.5 px-7 bg-transparent text-text-primary font-mono text-[13px] font-semibold no-underline border border-border-main cursor-pointer tracking-[1px] uppercase transition-all duration-200 hover:border-green hover:text-green hover:shadow-[0_0_20px_rgba(0,255,65,0.15)]"
          aria-label="Star DevLog on GitHub"
        >
          <Star size={16} aria-hidden="true" />
          Star on GitHub
        </a>
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
            <span className="text-text-primary font-medium">npm install -g devlog</span>
          </span>
          <span className="block mb-0.5">
            <span className="text-text-dim">  + devlog@1.0.4</span>
          </span>
          <span className="block mb-0.5">
            <span className="text-text-dim">  added 84 packages in 2.4s</span>
          </span>

          <div className="h-3" />

          <span className="block mb-0.5">
            <span className="text-green">$ </span>
            <span className="text-text-primary font-medium">devlog worklog --today</span>
          </span>

          <div className="h-3" />

          <span className="block mb-0.5">
            <span className="text-green">ACTIVITY REPORT [Today]:</span>
          </span>
          <span className="block mb-0.5">
            <span className="text-[#0a0a0a] bg-green px-1.5 rounded-[2px] text-[11px] font-semibold">[feat]</span>
            <span className="text-text-dim"> Implemented JWT rotation middleware</span>
          </span>
          <span className="block mb-0.5">
            <span className="text-[#0a0a0a] bg-[#febc2e] px-1.5 rounded-[2px] text-[11px] font-semibold">[refactor]</span>
            <span className="text-text-dim"> Optimized SQL queries in UserDAO (~40ms improvement)</span>
          </span>
          <span className="block mb-0.5">
            <span className="text-[#0a0a0a] bg-[#ff5f57] px-1.5 rounded-[2px] text-[11px] font-semibold">[fix]</span>
            <span className="text-text-dim"> Resolved WebSocket race condition #442</span>
          </span>
          <span className="block mb-0.5">
            <span className="text-[#0a0a0a] bg-[#58a6ff] px-1.5 rounded-[2px] text-[11px] font-semibold">[docs]</span>
            <span className="text-text-dim"> Updated API endpoints in README.md</span>
          </span>

          <div className="h-3" />

          <span className="block mb-0.5">
            <span className="text-text-dim">Total: 4 commits, 12 files changed, +456/-128 LOC</span>
          </span>

          <div className="h-3" />

          <span className="block mb-0.5">
            <span className="text-green">$ </span>
            <span className="text-text-primary font-medium">devlog generate --target=slack </span>
            <span className="inline-block w-2 h-4 bg-green animate-blink align-text-bottom" aria-hidden="true" />
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
        <p className="mt-3 text-sm text-text-secondary max-w-[460px]">
          Designed for the terminal native. No UI to click. No forms to fill. Just pure
          productivity extraction from your existing workflow.
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

  const handleCopy = () => {
    navigator.clipboard.writeText('npm i -g @devlog/cli')
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <section className="py-[120px] px-6 text-center" aria-labelledby="cta-title" id="pricing">
      <span className="text-green text-[40px] font-bold font-display" aria-hidden="true">&gt; </span>
      <h2 id="cta-title" className="font-display text-[32px] md:text-5xl font-bold tracking-[-1.5px] mb-10">
        Ready to commit?
      </h2>
      <button
        className="group inline-flex items-center gap-3 py-4 px-6 bg-bg-secondary border border-border-main rounded-md text-sm text-text-primary cursor-pointer transition-all duration-200 mb-6 hover:border-border-green hover:shadow-[0_0_20px_rgba(0,255,65,0.15)]"
        onClick={handleCopy}
        type="button"
        aria-label={copied ? 'Install command copied to clipboard' : 'Copy install command: npm i -g @devlog/cli'}
      >
        <span className="text-green" aria-hidden="true">$</span>
        <span>npm i -g @devlog/cli</span>
        {copied ? (
          <Check size={16} className="text-green" aria-hidden="true" />
        ) : (
          <Copy size={16} className="text-text-dim transition-colors duration-200 group-hover:text-green" aria-hidden="true" />
        )}
      </button>
      <span className="block mt-4 text-xs text-text-dim">Free &amp; Open Source. MIT License.</span>
    </section>
  )
}

function Footer() {
  return (
    <footer className="py-6 px-5 md:px-12 flex flex-col md:flex-row items-center justify-between gap-4 md:gap-0 text-center md:text-left border-t border-border-main text-[11px] text-text-dim">
      <span className="font-mono font-bold text-green text-[13px]">
        <span className="opacity-50" aria-hidden="true">&gt;_ </span>DevLog
      </span>
      <nav className="flex gap-6" aria-label="Footer navigation">
        <a href="#" className="text-text-dim no-underline uppercase tracking-[1px] text-[11px] transition-colors duration-200 hover:text-green">Privacy</a>
        <a href="#" className="text-text-dim no-underline uppercase tracking-[1px] text-[11px] transition-colors duration-200 hover:text-green">Terms</a>
        <a href="https://twitter.com" target="_blank" rel="noopener noreferrer" className="text-text-dim no-underline uppercase tracking-[1px] text-[11px] transition-colors duration-200 hover:text-green">X (Twitter)</a>
        <a href="https://github.com/ishaan812/devlog" target="_blank" rel="noopener noreferrer" className="text-text-dim no-underline uppercase tracking-[1px] text-[11px] transition-colors duration-200 hover:text-green">GitHub</a>
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
