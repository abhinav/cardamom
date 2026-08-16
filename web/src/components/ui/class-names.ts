import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function mergeClassNames(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

// shadcn registry components import this helper as `cn`; the alias keeps
// generated components compatible with Cardamom's descriptive name.
export { mergeClassNames as cn };
