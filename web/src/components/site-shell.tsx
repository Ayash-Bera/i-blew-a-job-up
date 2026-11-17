"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { Icon } from "./icons";

export function Header() {
  const pathname = usePathname();
  return <header className="site-header"><div className="header-inner">
    <Link href="/" className="wordmark" aria-label="FirstRole home"><span className="brand-mark"><Icon name="arrow" size={22} /></span>firstrole<span className="brand-dot">.</span></Link>
    <nav aria-label="Main navigation"><Link href="/" className={pathname === "/" ? "nav-link active" : "nav-link"}>Find a role</Link><Link href="/sources" className={pathname === "/sources" ? "nav-link active" : "nav-link"}>Sources & method</Link></nav>
    <span className="header-location"><span className="status-dot" />Built for your first step</span>
  </div></header>;
}

export function Footer() {
  return <footer className="site-footer"><div><Link href="/" className="footer-brand">firstrole.</Link><p>A clearer starting point for careers in India.</p></div><div className="footer-note">Always check the original post before applying.<br /><Link href="/sources">Read how listings are collected <Icon name="arrow" size={14} /></Link></div></footer>;
}
