import { useEffect, useId, useRef } from "react";
import type {
  MouseEvent,
  PointerEvent,
  ReactNode,
  SyntheticEvent,
} from "react";

type Props = {
  /** The sheet's heading, which also names the dialog. */
  readonly title: string;
  /** Escape, a click on the backdrop and the close button ask for this; the parent removes the sheet. */
  readonly onClose: () => void;
  readonly children: ReactNode;
};

/** Where focus starts: the first field, or the browser's choice in a sheet without one. */
const FIRST_FIELD = "input:not([type=hidden]), textarea, select";

/**
 * One sheet over the room (docs/UI.md, "Sheets and states inside the room"), on the native modal
 * <dialog>: the browser keeps focus inside it and makes the page behind inert. Focus starts in the
 * first field and goes back to where it was when the sheet goes away. The sheet never closes
 * itself: Escape, the backdrop and the close button call onClose, and the parent removes it.
 */
export function Sheet({ title, onClose, children }: Props) {
  const dialogRef = useRef<HTMLDialogElement>(null);
  const pressedBackdrop = useRef(false);
  const titleId = useId();

  useEffect(() => {
    const dialog = dialogRef.current;
    if (!dialog) return;
    const opener = document.activeElement;
    if (!dialog.open) dialog.showModal();
    dialog.querySelector<HTMLElement>(FIRST_FIELD)?.focus();
    return () => {
      dialog.close();
      if (opener instanceof HTMLElement && opener.isConnected) opener.focus();
    };
  }, []);

  function handleCancel(event: SyntheticEvent<HTMLDialogElement>) {
    event.preventDefault(); // Escape: the parent decides, by removing the sheet
    onClose();
  }

  function handleClose(event: SyntheticEvent<HTMLDialogElement>) {
    // A browser may close a dialog on a second Escape whatever the page says. A close that comes
    // late for a dialog that is open again (React's development double effect) is not one.
    if (!event.currentTarget.open) onClose();
  }

  // For events, the backdrop is the dialog itself: the sheet's body covers all the rest of it. A
  // press that starts inside the sheet and ends outside it (selecting text) is not a click on it.
  function handlePointerDown(event: PointerEvent<HTMLDialogElement>) {
    pressedBackdrop.current = event.target === event.currentTarget;
  }
  function handleClick(event: MouseEvent<HTMLDialogElement>) {
    if (pressedBackdrop.current && event.target === event.currentTarget) {
      onClose();
    }
  }

  return (
    <dialog
      ref={dialogRef}
      className="sheet-dialog"
      aria-labelledby={titleId}
      onCancel={handleCancel}
      onClose={handleClose}
      onPointerDown={handlePointerDown}
      onClick={handleClick}
    >
      <div className="sheet-body">
        <div className="sheet-head">
          <h2 id={titleId}>{title}</h2>
          <button
            type="button"
            className="icon-btn"
            aria-label="Close"
            onClick={onClose}
          >
            <svg
              viewBox="0 0 16 16"
              aria-hidden="true"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.5"
              strokeLinecap="round"
            >
              <path d="m4 4 8 8M12 4l-8 8" />
            </svg>
          </button>
        </div>
        {children}
      </div>
    </dialog>
  );
}
