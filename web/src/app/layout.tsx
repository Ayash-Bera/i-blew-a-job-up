import type { Metadata } from "next";
import { Header, Footer } from "@/components/site-shell";
import { themeBootstrapScript } from "@/lib/theme";
import "./globals.css";

export const metadata: Metadata = {
  title: { default: "I Blew a Job Up | Fresher jobs in India", template: "%s | I Blew a Job Up" },
  description: "Discover real fresher jobs and internships in India. Search public job listings by role, city, and work mode, with source links and eligibility evidence.",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return <html lang="en-IN" data-scroll-behavior="smooth" suppressHydrationWarning><head><script id="theme-preference" dangerouslySetInnerHTML={{ __html: themeBootstrapScript }} /></head><body><a href="#main-content" className="skip-link">Skip to content</a><Header />{children}<Footer /></body></html>;
}
