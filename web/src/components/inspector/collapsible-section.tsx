import { useReducer, type ReactNode } from "react";

interface CollapsibleSectionProps {
  title: string;
  initialOpen?: boolean;
  sectionColor?: string;
  children: ReactNode;
}

export function CollapsibleSection({
  title,
  initialOpen = false,
  sectionColor,
  children,
}: CollapsibleSectionProps) {
  const [open, toggleOpen] = useReducer((current: boolean) => !current, initialOpen);

  return (
    <div className="border border-border">
      <button
        type="button"
        onClick={toggleOpen}
        className="flex w-full items-center justify-between px-2 py-1.5 text-xs font-medium text-muted-foreground hover:text-foreground"
      >
        <span className="uppercase tracking-wider text-[10px]">{title}</span>
        <svg
          className={`h-3 w-3 transition-transform ${open ? "rotate-90" : ""}`}
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          strokeWidth={2}
        >
          <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
        </svg>
      </button>
      {open && (
        <div
          className="px-2 pb-2 pt-1"
          style={
            sectionColor
              ? ({
                  borderLeft: `2px solid ${sectionColor}`,
                  paddingLeft: "8px",
                } as React.CSSProperties)
              : undefined
          }
        >
          {children}
        </div>
      )}
    </div>
  );
}
