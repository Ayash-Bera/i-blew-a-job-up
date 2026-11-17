import type { Metadata } from "next";
import { Header, Footer } from "@/components/site-shell";
import "./globals.css";

export const metadata: Metadata = {
  title: { default: "FirstRole | Your first role starts here", template: "%s | FirstRole" },
  description: "Discover real fresher jobs and internships in India. Search public job listings by role, city, and work mode, with source links and eligibility evidence.",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return <html lang="en-IN"><body><a href="#main-content" className="skip-link">Skip to content</a><Header />{children}<Footer /></body></html>;
}
