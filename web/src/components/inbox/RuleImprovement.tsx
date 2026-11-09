import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { ApiError, post } from "@/api/client";
import {
  useNotificationRules,
  useSaveNotificationRules,
  type NotificationRule,
} from "@/api/notificationRules";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
type Improvement = {
  diagnosis: string;
  cause: string;
  risks: string;
  changes: { rule_id: string; text: string; explanation: string }[];
  proposal: NotificationRule[];
};
export function RuleImprovement({ questionID }: { questionID?: string }) {
  const [open, setOpen] = useState(false);
  return (
    <div className="mt-3 space-y-2">
      <Button size="sm" variant="outline" onClick={() => setOpen(!open)}>
        Improve rules
      </Button>
      {open && <Form questionID={questionID} />}
    </div>
  );
}
function Form({ questionID }: { questionID?: string }) {
  const qc = useQueryClient(),
    rules = useNotificationRules(),
    save = useSaveNotificationRules();
  const [kind, setKind] = useState("unclear_rule"),
    [instructions, setInstructions] = useState("");
  const suggest = useMutation({
    mutationFn: () =>
      post<Improvement>("/api/notifications/rules/improvements", {
        base_revision: rules.data?.[0]?.revision,
        question_id: questionID ?? "",
        kind,
        instructions,
      }),
    onError: (e) => {
      if (e instanceof ApiError && e.status === 409)
        void qc.invalidateQueries({ queryKey: ["notification-rules"] });
    },
  });
  const [proposalBase, setProposalBase] = useState(0);
  return (
    <section
      className="space-y-3 rounded border p-3"
      aria-label="Improve notification rules"
    >
      <p className="text-xs">
        Describe the behavior you want. Suggestions do not change rules until
        you review and save them.
      </p>
      <select
        value={kind}
        disabled={suggest.isPending}
        onChange={(e) => setKind(e.target.value)}
        className="rounded bg-background p-2"
      >
        <option value="unclear_rule">Rule unclear</option>
        <option value="already_handled">Already handled</option>
        <option value="not_relevant">Not relevant</option>
        <option value="wrong_task">Wrong task</option>
        <option value="show_similar">Show similar updates</option>
      </select>
      <Textarea
        aria-label="Desired notification behavior"
        maxLength={4000}
        value={instructions}
        disabled={suggest.isPending}
        onChange={(e) => setInstructions(e.target.value)}
      />
      <Button
        size="sm"
        disabled={
          !instructions.trim() || !rules.data?.length || suggest.isPending
        }
        onClick={() => {
          setProposalBase(rules.data![0].revision);
          suggest.mutate();
        }}
      >
        {suggest.isPending ? "Diagnosing…" : "Suggest rule changes"}
      </Button>
      {suggest.data && (
        <div className="space-y-2 text-sm">
          <p>{suggest.data.diagnosis}</p>
          <p>{suggest.data.risks}</p>
          {suggest.data.changes.map((c) => (
            <div key={c.rule_id}>
              <strong>{c.rule_id}</strong>
              <p>{c.text}</p>
              <p>{c.explanation}</p>
            </div>
          ))}
          {!!suggest.data.changes.length && (
            <Button
              size="sm"
              disabled={
                save.isPending || rules.data?.[0]?.revision !== proposalBase
              }
              onClick={() =>
                save.mutate({
                  base_revision: proposalBase,
                  rules: suggest.data.proposal,
                })
              }
            >
              Save suggested rules
            </Button>
          )}
        </div>
      )}
      {save.data && (
        <p role="status">Saved rule revision {save.data.revision}.</p>
      )}
      {(suggest.error || save.error || rules.error) && (
        <p role="alert">
          {(suggest.error || save.error || rules.error)?.message}
        </p>
      )}
    </section>
  );
}
