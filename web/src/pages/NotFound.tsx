import { Link } from "../components/Link";

export function NotFound() {
  return (
    <div className="wrap page">
      <h1>There is no page here</h1>
      <p className="lead">
        The address may be mistyped. A project's room lives at /p/owner/repo.
      </p>
      <div className="actions">
        <Link href="/" className="btn ghost">
          Back to the start
        </Link>
      </div>
    </div>
  );
}
