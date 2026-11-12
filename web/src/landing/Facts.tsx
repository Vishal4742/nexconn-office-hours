// Three big numbers with one line each, and three short benefits (docs/UI.md S1). Every line
// says what this build does: NFR-PERF-1 (a queue change reaches every panel within 1 s),
// FR-FAQ-1 to 3, FR-SESS-1, 4 and 7, FR-HAND-1.

const FACTS = [
  {
    figure: "≤1s",
    said: "Within a second",
    line: "from your Next to every screen in the room",
  },
  {
    figure: "1×",
    said: "Once",
    line: "each question answered once, then linked for good",
  },
  {
    figure: "0",
    said: "Zero",
    line: "installs, bots or servers for you to babysit",
  },
] as const;

const BENEFITS = [
  {
    title: "A start and an end",
    text: "You pick a slot, say 45 minutes. When you end it, waiting hands move to your next session, so nobody is left wondering.",
  },
  {
    title: "Quiet between sessions",
    text: "Hands only go up while you're live, so between sessions nobody is waiting on you.",
  },
  {
    title: "Answers people can find",
    text: "Every answer gets a public page that loads signed out, with its own link for issues, docs and search.",
  },
] as const;

export function Facts() {
  return (
    <section className="land-section" aria-labelledby="land-why-h">
      <h2 id="land-why-h" className="land-h2">
        Made for people who maintain in their evenings
      </h2>
      <ul className="land-facts">
        {FACTS.map((fact) => (
          <li key={fact.said} className="land-fact">
            <span className="land-fact-figure" aria-hidden="true">
              {fact.figure}
            </span>
            <span className="sr-only">{fact.said}: </span>
            <span className="land-fact-line">{fact.line}</span>
          </li>
        ))}
      </ul>
      <div className="land-benefits">
        {BENEFITS.map((benefit) => (
          <div key={benefit.title}>
            <h3>{benefit.title}</h3>
            <p>{benefit.text}</p>
          </div>
        ))}
      </div>
    </section>
  );
}
