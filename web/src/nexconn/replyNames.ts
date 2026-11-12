// A workaround for one Chat UI bug, kept apart so it is easy to delete.
//
// Chat UI 26.2.5 draws a reply with the quoted author's raw user id in front of the quote
// ("Reply gh12345678: ..."), for every reader: its bubble prints `content.referMsgSenderId` and
// never looks the name up. This hands the bubble's own render a copy of the message whose
// content carries the name in that field. Nothing in Chat UI's state is changed, and if the
// element or its render ever goes away, replies show the id again and nothing else breaks.
//
// ponytail: delete this file when Chat UI prints names in replies (check after each upgrade).
import { nameOf } from "./roster";

const REPLY_BUBBLE = "nc-reference-message";

type ReplyBubble = {
  readonly message?: {
    readonly content?: { readonly referMsgSenderId?: unknown };
  };
};

function withAuthorName(render: () => unknown) {
  return function (this: ReplyBubble): unknown {
    const message = this.message;
    const authorId = message?.content?.referMsgSenderId;
    const name = typeof authorId === "string" ? nameOf(authorId) : null;
    if (!message || !name) return render.call(this);

    const named = new Proxy(message, {
      get: (target, key) =>
        key === "content"
          ? { ...target.content, referMsgSenderId: name }
          : Reflect.get(target, key, target),
    });
    const bubble = new Proxy(this, {
      get: (target, key) =>
        key === "message" ? named : Reflect.get(target, key, target),
    });
    return render.call(bubble);
  };
}

/** Makes reply bubbles show the quoted author's name. Safe to call once Chat UI has started. */
export function showNamesInReplies(): void {
  void customElements.whenDefined(REPLY_BUBBLE).then((bubble) => {
    const render: unknown = bubble.prototype.render;
    if (typeof render !== "function") return;
    bubble.prototype.render = withAuthorName(render as () => unknown);
  });
}
