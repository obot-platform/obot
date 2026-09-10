import React, {
  type FormEvent,
  type ReactNode,
  useId,
  useRef,
  useState,
} from "react";
import {
  DEFAULT_OBOT_URL,
  normalizeObotUrl,
  saveObotUrl,
  useObotUrl,
} from "../../lib/obotUrl";
import styles from "./styles.module.css";

type Props = {
  mobile?: boolean;
};

export default function ObotUrlNavbarItem({ mobile = false }: Props): ReactNode {
  const obotUrl = useObotUrl();
  const dialogRef = useRef<HTMLDialogElement>(null);
  const titleId = useId();
  const [draft, setDraft] = useState(obotUrl);
  const [error, setError] = useState("");

  function openDialog(): void {
    setDraft(obotUrl);
    setError("");
    dialogRef.current?.showModal();
  }

  function applyUrl(event: FormEvent<HTMLFormElement>): void {
    event.preventDefault();
    try {
      const normalized = normalizeObotUrl(draft);
      saveObotUrl(normalized);
      setDraft(normalized);
      setError("");
      dialogRef.current?.close();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Enter a valid URL.");
    }
  }

  const trigger = (
    <button
      type="button"
      className={
        mobile
          ? `menu__link ${styles.trigger}`
          : `navbar__item navbar__link ${styles.trigger}`
      }
      onClick={openDialog}
    >
      Set Obot URL
    </button>
  );

  const dialog = (
    <dialog
      ref={dialogRef}
      className={styles.dialog}
      aria-labelledby={titleId}
      onClick={(event) => {
        if (event.target === event.currentTarget) {
          event.currentTarget.close();
        }
      }}
    >
      <form className={styles.form} onSubmit={applyUrl}>
        <h2 id={titleId} className={styles.title}>
          Obot URL
        </h2>
        <p className={styles.description}>
          Code examples use this URL in place of {DEFAULT_OBOT_URL}.
        </p>
        <label className={styles.label} htmlFor={`${titleId}-input`}>
          Base URL
        </label>
        <input
          id={`${titleId}-input`}
          className={styles.input}
          type="url"
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          aria-describedby={error ? `${titleId}-error` : undefined}
          required
        />
        {error && (
          <p id={`${titleId}-error`} className={styles.error} role="alert">
            {error}
          </p>
        )}
        <div className={styles.presets}>
          <button
            type="button"
            className="button button--secondary button--sm"
            onClick={() => setDraft("http://localhost:8080")}
          >
            Use localhost
          </button>
          <button
            type="button"
            className="button button--secondary button--sm"
            onClick={() => setDraft(DEFAULT_OBOT_URL)}
          >
            Use default
          </button>
        </div>
        <div className={styles.actions}>
          <button
            type="button"
            className="button button--secondary"
            onClick={() => dialogRef.current?.close()}
          >
            Cancel
          </button>
          <button type="submit" className="button button--primary">
            Apply
          </button>
        </div>
      </form>
    </dialog>
  );

  return mobile ? (
    <li className="menu__list-item">
      {trigger}
      {dialog}
    </li>
  ) : (
    <>
      {trigger}
      {dialog}
    </>
  );
}
