import { useMemo, useState } from 'react'
import { Search, Home, Github, BookOpen } from 'lucide-react'
import { DOC_SECTIONS } from './docsContent'

function escapeRegex(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

function highlightText(text, query) {
  if (!query.trim()) return text
  const regex = new RegExp(`(${escapeRegex(query.trim())})`, 'ig')
  const parts = text.split(regex)
  return parts.map((part, idx) =>
    part.toLowerCase() === query.trim().toLowerCase() ? (
      <mark
        key={`${part}-${idx}`}
        className="bg-green-glow text-green px-0.5 rounded-sm"
      >
        {part}
      </mark>
    ) : (
      part
    ),
  )
}

function splitParam(param) {
  const idx = param.indexOf(':')
  if (idx === -1) {
    return { name: param, description: '' }
  }
  return {
    name: param.slice(0, idx).trim(),
    description: param.slice(idx + 1).trim(),
  }
}

function CodeBlock({ lines }) {
  const renderToken = (token, idx) => {
    if (token.startsWith('--') || (token.startsWith('-') && token.length > 1)) {
      return (
        <span key={`${token}-${idx}`} className="text-[#58a6ff]">
          {token}
        </span>
      )
    }
    if (
      token.startsWith('~/') ||
      token.startsWith('/') ||
      token.startsWith('./') ||
      token.includes('/')
    ) {
      return (
        <span key={`${token}-${idx}`} className="text-[#febc2e]">
          {token}
        </span>
      )
    }
    if (token.startsWith('"') || token.endsWith('"') || token.startsWith("'") || token.endsWith("'")) {
      return (
        <span key={`${token}-${idx}`} className="text-[#79c0ff]">
          {token}
        </span>
      )
    }
    if (idx === 0 && token === 'devlog') {
      return (
        <span key={`${token}-${idx}`} className="text-green font-semibold">
          {token}
        </span>
      )
    }
    return (
      <span key={`${token}-${idx}`} className="text-text-primary">
        {token}
      </span>
    )
  }

  const renderLine = (line, lineIndex) => {
    const tokens = line.trim().split(/\s+/).filter(Boolean)
    return (
      <div key={`${line}-${lineIndex}`} className="leading-[1.9]">
        <span className="text-green mr-2" aria-hidden="true">
          $
        </span>
        {tokens.map((token, tokenIndex) => (
          <span key={`${token}-${tokenIndex}`}>
            {renderToken(token, tokenIndex)}
            {tokenIndex < tokens.length - 1 ? <span className="text-text-dim"> </span> : null}
          </span>
        ))}
      </div>
    )
  }

  return (
    <div className="mt-4 rounded-md border border-border-main overflow-hidden bg-bg-secondary">
      <div className="flex items-center gap-2 px-3 py-2 border-b border-border-main bg-bg-card">
        <span className="w-2 h-2 rounded-full bg-[#ff5f57]" aria-hidden="true" />
        <span className="w-2 h-2 rounded-full bg-[#febc2e]" aria-hidden="true" />
        <span className="w-2 h-2 rounded-full bg-[#28c840]" aria-hidden="true" />
        <span className="ml-2 text-[10px] tracking-[1px] uppercase text-text-dim">Command</span>
      </div>
      <pre className="p-4 overflow-x-auto text-xs">
        <code>{lines.map((line, lineIndex) => renderLine(line, lineIndex))}</code>
      </pre>
    </div>
  )
}

function DocsSection({ section, query }) {
  const hasCommands = Array.isArray(section.commands) && section.commands.length > 0

  return (
    <article
      className="bg-bg-card border border-border-main rounded-md p-5 md:p-6 scroll-mt-24"
      id={section.id}
    >
      <h2 className="font-display text-2xl font-semibold tracking-[-0.4px] text-text-primary text-balance">
        {highlightText(section.title, query)}
      </h2>
      <p className="mt-2 text-sm text-text-secondary">
        {highlightText(section.summary, query)}
      </p>
      <div className="mt-4 space-y-2 text-[13px] leading-[1.8] text-text-secondary">
        {section.body.map((line) => (
          <p key={line} className="wrap-break-word">
            {highlightText(line, query)}
          </p>
        ))}
      </div>
      {section.params?.length ? (
        <div className="mt-4">
          <h3 className="font-mono text-xs uppercase tracking-[1px] text-green mb-2">
            Parameters
          </h3>
          <ul className="space-y-2 text-[12px] leading-[1.8] text-text-secondary">
            {section.params.map((param) => (
              <li key={param} className="wrap-break-word">
                <span className="text-text-primary font-medium">{highlightText(splitParam(param).name, query)}</span>
                {splitParam(param).description ? (
                  <span className="text-text-secondary">: {highlightText(splitParam(param).description, query)}</span>
                ) : null}
              </li>
            ))}
          </ul>
        </div>
      ) : null}

      {hasCommands ? (
        <div className="mt-5 space-y-4">
          {section.commands.map((command) => (
            <article
              id={command.id}
              key={command.id}
              className="scroll-mt-24 border border-border-main rounded-md p-4 bg-bg-secondary/80 hover:border-border-green transition-colors duration-200"
            >
              <div className="inline-flex items-center rounded-full px-2 py-0.5 text-[10px] uppercase tracking-[1px] bg-green-glow text-green border border-border-green mb-2">
                CLI Command
              </div>
              <h3 className="font-display text-xl text-text-primary">{highlightText(command.title, query)}</h3>
              <p className="mt-1 text-sm text-text-secondary">{highlightText(command.summary, query)}</p>
              <p className="mt-3 text-[12px] text-text-dim flex flex-wrap gap-2 items-center">
                <span className="uppercase tracking-[1px] text-green">Usage</span>
                <code className="text-text-primary bg-bg-card border border-border-main rounded px-2 py-1">{command.usage}</code>
              </p>
              {command.params?.length ? (
                <div className="mt-3">
                  <h4 className="font-mono text-xs uppercase tracking-[1px] text-green mb-2">
                    Parameters
                  </h4>
                  <ul className="space-y-2 text-[12px] leading-[1.8] text-text-secondary">
                    {command.params.map((param) => (
                      <li key={param} className="wrap-break-word">
                        <span className="text-text-primary font-medium">{highlightText(splitParam(param).name, query)}</span>
                        {splitParam(param).description ? (
                          <span className="text-text-secondary">: {highlightText(splitParam(param).description, query)}</span>
                        ) : null}
                      </li>
                    ))}
                  </ul>
                </div>
              ) : null}
              {command.examples?.length ? (
                <CodeBlock lines={command.examples} />
              ) : null}
            </article>
          ))}
        </div>
      ) : (
        <CodeBlock lines={section.code} />
      )}
    </article>
  )
}

export default function DocsPage() {
  const [query, setQuery] = useState('')

  const filteredSections = useMemo(() => {
    const normalized = query.trim().toLowerCase()
    if (!normalized) return DOC_SECTIONS

    return DOC_SECTIONS.filter((section) => {
      const searchableText = [
        section.title,
        section.summary,
        ...section.body,
        ...(section.params ?? []),
        ...section.code,
        ...(section.commands ?? []).flatMap((command) => [
          command.title,
          command.summary,
          command.usage,
          ...(command.params ?? []),
          ...(command.examples ?? []),
        ]),
      ]
        .join(' ')
        .toLowerCase()

      if (!searchableText.includes(normalized)) return false

      if (!section.commands) return true

      const filteredCommands = section.commands.filter((command) => {
        const commandText = [
          command.title,
          command.summary,
          command.usage,
          ...(command.params ?? []),
          ...(command.examples ?? []),
        ]
          .join(' ')
          .toLowerCase()

        return commandText.includes(normalized)
      })

      return filteredCommands.length > 0 || searchableText.includes(normalized)
    })
    .map((section) => {
      if (!section.commands || !normalized) return section

      const filteredCommands = section.commands.filter((command) => {
        const commandText = [
          command.title,
          command.summary,
          command.usage,
          ...(command.params ?? []),
          ...(command.examples ?? []),
        ]
          .join(' ')
          .toLowerCase()
        return commandText.includes(normalized)
      })

      return {
        ...section,
        commands: filteredCommands,
      }
    })
  }, [query])

  return (
    <>
      <a
        href="#docs-main"
        className="sr-only focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-200 focus:bg-green focus:text-bg-primary focus:px-4 focus:py-2 focus:rounded focus:font-mono focus:text-sm"
      >
        Skip to documentation content
      </a>

      <nav
        className="fixed top-0 inset-x-0 z-100 h-14 px-5 md:px-12 bg-[rgba(10,10,10,0.85)] backdrop-blur-md border-b border-border-main flex items-center justify-between"
        aria-label="Documentation navigation"
      >
        <a
          href="/"
          className="font-mono text-base font-bold text-green no-underline flex items-center gap-2 tracking-[-0.5px]"
          aria-label="Go to home page"
        >
          <span className="opacity-50" aria-hidden="true">
            &gt;_
          </span>
          DevLog
        </a>
        <div className="flex items-center gap-4">
          <a
            href="/"
            className="inline-flex items-center gap-1 font-mono text-xs text-text-secondary no-underline uppercase tracking-[1px] transition-colors duration-200 hover:text-green focus-visible:outline-2 focus-visible:outline-green focus-visible:outline-offset-2 rounded-sm px-1.5 py-1"
          >
            <Home size={14} aria-hidden="true" />
            Home
          </a>
          <a
            href="https://github.com/ishaan812/devlog"
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1 font-mono text-xs text-text-secondary no-underline uppercase tracking-[1px] transition-colors duration-200 hover:text-green focus-visible:outline-2 focus-visible:outline-green focus-visible:outline-offset-2 rounded-sm px-1.5 py-1"
            aria-label="Open DevLog GitHub repository"
          >
            <Github size={14} aria-hidden="true" />
            GitHub
          </a>
        </div>
      </nav>

      <main id="docs-main" className="max-w-[1200px] mx-auto px-3 md:px-6 pt-20 md:pt-24 pb-20">
        <header className="mb-8 md:mb-10 px-2 md:px-4 lg:pl-6">
          <div className="inline-flex items-center gap-2 py-1 px-3 rounded-full bg-[rgba(0,255,65,0.08)] border border-border-green text-[11px] text-green uppercase tracking-[1.5px]">
            <BookOpen size={13} aria-hidden="true" />
            Documentation
          </div>
          <h1 className="mt-4 font-display text-4xl md:text-5xl font-bold tracking-[-1px] text-text-primary text-balance">
            DevLog Docs
          </h1>
          <p className="mt-3 text-sm leading-[1.8] text-text-secondary max-w-2xl">
            Minimal, practical docs based on the project README. Search across sections to find commands and setup steps quickly.
          </p>
        </header>

        <section className="mb-8 px-2 md:px-4 lg:pl-6" aria-labelledby="docs-search-title">
          <h2 id="docs-search-title" className="sr-only">
            Search Documentation
          </h2>
          <label
            htmlFor="docs-search"
            className="block text-xs uppercase tracking-[1px] text-text-dim mb-2"
          >
            Search This Page
          </label>
          <div className="relative focus-within:[box-shadow:0_0_0_1px_rgba(0,255,65,0.5)] rounded-md">
            <Search
              size={15}
              className="absolute left-3 top-1/2 -translate-y-1/2 text-text-dim"
              aria-hidden="true"
            />
            <input
              id="docs-search"
              name="docs-search"
              type="search"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="Try: install, worklog, obsidian…"
              autoComplete="off"
              className="w-full bg-bg-secondary border border-border-main rounded-md pl-10 pr-3 py-3 text-sm text-text-primary placeholder:text-text-dim focus-visible:outline-2 focus-visible:outline-green focus-visible:outline-offset-2"
              aria-describedby="docs-search-hint"
            />
          </div>
          <p id="docs-search-hint" className="mt-2 text-xs text-text-dim">
            {filteredSections.length} section{filteredSections.length === 1 ? '' : 's'} matched
          </p>
        </section>

        {filteredSections.length === 0 ? (
          <section className="bg-bg-card border border-border-main rounded-md p-6">
            <h2 className="font-display text-xl text-text-primary">No Results</h2>
            <p className="mt-2 text-sm text-text-secondary">
              Try broader terms like install, ingest, worklog, export, or profile.
            </p>
          </section>
        ) : (
          <div className="grid grid-cols-1 lg:grid-cols-[260px_minmax(0,1fr)] gap-5 lg:gap-6 items-start">
            <aside className="lg:sticky lg:top-20 lg:h-[calc(100vh-100px)] lg:overflow-y-auto border-r border-border-main pr-4">
              <nav aria-label="Documentation section navigation">
                <ul className="space-y-1 text-[12px]">
                  {filteredSections.map((section) => (
                    <li key={section.id}>
                      <a
                        href={`#${section.id}`}
                        className="block no-underline text-text-secondary hover:text-green focus-visible:outline-2 focus-visible:outline-green focus-visible:outline-offset-2 rounded-sm px-2 py-1.5 transition-colors duration-200"
                      >
                        {section.title}
                      </a>
                      {section.id === 'cli-documentation' && section.commands?.length ? (
                        <ul className="mt-1 ml-2 border-l border-border-main pl-2 space-y-1">
                          {section.commands.map((command) => (
                            <li key={command.id}>
                              <a
                                href={`#${command.id}`}
                                className="block no-underline text-text-dim hover:text-green focus-visible:outline-2 focus-visible:outline-green focus-visible:outline-offset-2 rounded-sm px-2 py-1 transition-colors duration-200"
                              >
                                {command.title}
                              </a>
                            </li>
                          ))}
                        </ul>
                      ) : null}
                    </li>
                  ))}
                </ul>
              </nav>
            </aside>

            <section className="space-y-5 min-w-0 lg:pl-4" aria-label="Documentation sections">
              {filteredSections.map((section) => (
                <DocsSection key={section.id} section={section} query={query} />
              ))}
            </section>
          </div>
        )}
      </main>
    </>
  )
}
