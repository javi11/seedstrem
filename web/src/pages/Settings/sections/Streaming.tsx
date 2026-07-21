import { SectionProps } from "../types";
import { SectionCard, NumberField } from "../fields";

export function Streaming({ config, update }: SectionProps) {
  return (
    <SectionCard title="Streaming">
      <div className="grid gap-4 sm:grid-cols-2">
        <NumberField
          label="Wait timeout for missing pieces (seconds)"
          min={5}
          value={config.stream.wait_timeout_seconds}
          onChange={(v) => update((c) => (c.stream.wait_timeout_seconds = v))}
        />
        <NumberField
          label="Read chunk size (MiB)"
          min={1}
          value={Math.max(1, Math.round(config.stream.read_chunk / 1048576))}
          onChange={(v) => update((c) => (c.stream.read_chunk = Math.max(1, v) * 1048576))}
        />
      </div>
    </SectionCard>
  );
}
