import { useState } from "react";
import { Search, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

import type { IssueFilterNavigation } from "@/collection-route.ts";
import type { IssueFilters } from "@/issue-collection.ts";

interface CollectionSearchControlProps {
  filters: IssueFilters;
  setFilters: (
    filters: IssueFilters,
    navigation: IssueFilterNavigation,
  ) => void;
}

export function CollectionSearchControl(
  props: CollectionSearchControlProps,
) {
  const [editing, setEditing] = useState(false);

  return (
    <CollectionSearchControlView
      {...props}
      editing={editing}
      beginEditing={() => setEditing(true)}
      endEditing={() => setEditing(false)}
    />
  );
}

interface CollectionSearchControlViewProps
  extends CollectionSearchControlProps {
  editing: boolean;
  beginEditing: () => void;
  endEditing: () => void;
}

export function CollectionSearchControlView(
  props: CollectionSearchControlViewProps,
) {
  const query = props.filters.query.trim();
  const active = props.editing || query !== "";

  return (
    <div
      className={`collection-search-control${
        active ? " collection-search-control-active" : ""
      }${props.editing ? " collection-search-control-editing" : ""}`}
      onBlur={(event) => {
        if (
          props.editing &&
          !event.currentTarget.contains(event.relatedTarget)
        ) {
          props.endEditing();
        }
      }}
    >
      {props.editing ? (
        <>
          <Search className="collection-search-icon" aria-hidden="true" />
          <Input
            autoFocus
            className="collection-search-input"
            type="search"
            aria-label="Search issue titles"
            value={props.filters.query}
            placeholder="Search issue titles"
            onChange={(event) =>
              props.setFilters(
                {
                  ...props.filters,
                  query: event.currentTarget.value,
                },
                "replace",
              )
            }
          />
        </>
      ) : (
        <Button
          type="button"
          className="collection-search-trigger"
          variant="outline"
          size="icon"
          aria-label="Search issues"
          title="Search issues"
          onClick={props.beginEditing}
        >
          <Search aria-hidden="true" />
          {query !== "" && (
            <span className="collection-search-query" title={query}>
              {query}
            </span>
          )}
        </Button>
      )}
      {active && (
        <Button
          type="button"
          className="collection-search-clear"
          variant="ghost"
          size="icon-sm"
          aria-label="Clear search"
          title="Clear search"
          onClick={() =>
            props.setFilters(
              {
                ...props.filters,
                query: "",
              },
              "replace",
            )}
        >
          <X aria-hidden="true" />
        </Button>
      )}
    </div>
  );
}
