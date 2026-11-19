"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { Icon } from "./icons";
import { ThemeControl } from "./theme-control";

export function Header() {
  const pathname = usePathname();
  return <header className="site-header"><div className="header-inner">
    <Link href="/" className="wordmark" aria-label="I Blew a Job Up home"><span className="brand-mark"><Icon name="arrow" size={22} /></span><span className="brand-name">I Blew a Job Up<span className="brand-dot">.</span></span></Link>
    <nav aria-label="Main navigation"><Link href="/" className={pathname === "/" ? "nav-link active" : "nav-link"}>Find a role</Link><Link href="/sources" className={pathname === "/sources" ? "nav-link active" : "nav-link"}>Sources & method</Link></nav>
    <div className="header-actions"><span className="header-location"><span className="status-dot" />Built for your first step</span><ThemeControl /></div>
  </div></header>;
}

export function Footer() {
  return <footer className="site-footer"><div><Link href="/" className="footer-brand">I Blew a Job Up.</Link><p>A clearer starting point for careers in India.</p></div><div className="footer-note">Always check the original post before applying.<br /><Link href="/sources">Read how listings are collected <Icon name="arrow" size={14} /></Link></div></footer>;
}
