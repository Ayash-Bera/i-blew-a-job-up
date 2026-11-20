"use client";

import Image from "next/image";
import { useState } from "react";
import { companyLogo, companyInitials } from "@/lib/company-logos";

export function CompanyLogo({ company }: { company: string }) {
  const logo = companyLogo(company);
  const [failedSource, setFailedSource] = useState<string | null>(null);
  const showLogo = logo && failedSource !== logo;
  const color = [...company].reduce((sum, char) => sum + char.charCodeAt(0), 0) % 5;

  return <div className={`company-mark ${showLogo ? "company-logo" : `company-color-${color}`}`} aria-hidden="true">
    {showLogo
      ? <Image src={logo} alt="" width={32} height={32} unoptimized onError={() => setFailedSource(logo)} />
      : companyInitials(company)}
  </div>;
}
