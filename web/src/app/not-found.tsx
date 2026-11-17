import Link from "next/link";

export default function NotFound() {
  return <main id="main-content" className="page-width standalone-state"><span className="eyebrow">404 / PAGE NOT FOUND</span><h1>This path ends here.</h1><p>Your next role could be one click away.</p><Link className="button button-dark" href="/">Find a role</Link></main>;
}
