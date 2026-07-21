import { Config } from "../../api";

export interface SectionProps {
  config: Config;
  update: (fn: (c: Config) => void) => void;
}

export interface SectionDef {
  id: string;
  label: string;
  icon: string;
  group: string;
  restart?: boolean;
}
