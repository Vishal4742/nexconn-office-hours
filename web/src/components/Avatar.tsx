type Props = {
  readonly name: string;
  readonly url: string;
  readonly size?: "sm" | "md";
};

function initials(name: string): string {
  return name
    .split(/\s+/)
    .map((word) => word[0] ?? "")
    .join("")
    .slice(0, 2)
    .toUpperCase();
}

/** Someone's picture, or their initials when they have none. The name next to it says who it is. */
export function Avatar({ name, url, size = "md" }: Props) {
  const className = size === "sm" ? "av sm" : "av";
  if (!url) {
    return (
      <span className={className} aria-hidden="true">
        {initials(name)}
      </span>
    );
  }
  return (
    <img className={className} src={url} alt="" referrerPolicy="no-referrer" />
  );
}
