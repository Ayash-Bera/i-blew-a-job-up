"use client";

export default function ErrorPage({ reset }: { reset: () => void }) {
  return <main id="main-content" className="page-width standalone-state"><span className="eyebrow">SOMETHING WENT WRONG</span><h1>Let&apos;s try that again.</h1><p>The page could not load. Your search is still in the address bar.</p><button className="button button-dark" onClick={reset}>Reload page</button></main>;
}
