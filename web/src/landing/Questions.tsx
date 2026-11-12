import { Caret } from "./icons";

// "Before you host": the demo's questions, each answer true of this build (docs/SRS.md):
// FR-AUTH-1, FR-PROJ-1, FR-FAQ-3, FR-SESS-4 and 7, FR-FAQ-5, and the stack in ARCHITECTURE.md.
const QUESTIONS = [
  {
    question: "Does it cost anything?",
    answer:
      "No. Office Hours is free for open source projects. You sign in with GitHub and register a repo you maintain.",
  },
  {
    question: "What can Office Hours see in my GitHub account?",
    answer:
      "Your public profile: id, login, name and avatar. It asks for read:user only and never writes to your repos.",
  },
  {
    question: "Who can host a room?",
    answer:
      "Anyone with admin or maintain rights on the repo. Office Hours checks that with GitHub when you register it, and again when you start a session.",
  },
  {
    question: "Do contributors need an account?",
    answer:
      "To raise a hand, yes: a GitHub sign in. Reading the FAQ needs nothing at all.",
  },
  {
    question: "What happens to questions we don't reach?",
    answer:
      "When you end the session they move to your next one, at the front of the line, so their authors don't have to ask again. You can untick that when you end.",
  },
  {
    question: "Can a question turn into an issue?",
    answer:
      "Yes. The question being answered and every FAQ entry have an Open GitHub issue link, with the title and details filled in. Office Hours writes nothing to GitHub itself.",
  },
  {
    question: "What is it built on?",
    answer:
      "The room runs on the Nexconn Chat SDK and Chat UI. The queue and the FAQ live in Postgres behind a small Go server.",
  },
] as const;

/** Plain questions and answers: native <details>, so they open by keyboard and work without script. */
export function Questions() {
  return (
    <section className="land-section" aria-labelledby="land-qa-h">
      <h2 id="land-qa-h" className="land-h2">
        Before you host
      </h2>
      <div className="land-qa">
        {QUESTIONS.map(({ question, answer }) => (
          <details key={question}>
            <summary>
              {question}
              <Caret />
            </summary>
            <p>{answer}</p>
          </details>
        ))}
      </div>
    </section>
  );
}
