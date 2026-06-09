export type SamplingOptions = {
  readonly temperature?: number;
  readonly topP?: number;
  readonly seed?: number;
  readonly presencePenalty?: number;
  readonly frequencyPenalty?: number;
};

type SamplingOptionInput = {
  readonly [Key in keyof SamplingOptions]?: SamplingOptions[Key] | undefined;
};

export function samplingOptionsFromEntries(options: SamplingOptionInput): SamplingOptions {
  const entries = Object.entries(options).filter(([, value]) => value !== undefined);
  return Object.fromEntries(entries);
}

export function hasSamplingOptions(options: SamplingOptions): boolean {
  return Object.keys(options).length > 0;
}

export function samplingRequestParams(options: SamplingOptions): Record<string, number> {
  return Object.fromEntries([
    ...entry("temperature", options.temperature),
    ...entry("top_p", options.topP),
    ...entry("seed", options.seed),
    ...entry("presence_penalty", options.presencePenalty),
    ...entry("frequency_penalty", options.frequencyPenalty)
  ]);
}

function entry(key: string, value: number | undefined): readonly (readonly [string, number])[] {
  return value === undefined ? [] : [[key, value]];
}
