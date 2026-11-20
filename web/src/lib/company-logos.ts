const logos: Record<string, string> = {
  "66degrees": "66degrees.png",
  "adobe": "adobe.png",
  "certa": "certa.png",
  "cloudsek": "cloudsek.png",
  "composio": "composio.png",
  "drivetrain": "drivetrain-icon.png",
  "dun & bradstreet": "dun-bradstreet.png",
  "emergent labs": "emergent-labs.png",
  "forma": "forma.png",
  "fourkites": "fourkites.png",
  "gainsight": "gainsight.svg",
  "hackerrank": "hackerrank.png",
  "hevo data": "hevo-data.png",
  "hp": "hp.png",
  "instawork": "instawork.png",
  "marriott international": "marriott.svg",
  "medtronic": "medtronic.png",
  "motorola solutions": "motorola-solutions.png",
  "paytm": "paytm.svg",
  "qad": "qad.png",
  "rubrik": "rubrik.png",
  "sarvam": "sarvam.svg",
  "stripe": "stripe.png",
};

export function companyLogo(company: string): string | undefined {
  const name = company.trim().replace(/\s+/g, " ").toLowerCase();
  return Object.hasOwn(logos, name) ? `/company-logos/${logos[name]}` : undefined;
}

export function companyInitials(company: string): string {
  const name = company.trim();
  if (/^[A-Z0-9]{1,3}$/.test(name)) return name;
  return name.split(/\s+/).map((part) => [...part][0]).slice(0, 2).join("").toUpperCase() || "?";
}
