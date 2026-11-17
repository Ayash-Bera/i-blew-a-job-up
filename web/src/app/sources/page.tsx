import type { Metadata } from "next";
import { SourcesPage } from "@/components/sources-page";

export const metadata: Metadata = { title: "Sources & method" };

export default function Sources() { return <SourcesPage />; }
