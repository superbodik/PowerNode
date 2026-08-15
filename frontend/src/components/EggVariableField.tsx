import type { EggVariable } from '../types';

interface Props {
  variable: EggVariable;
  value: string;
  onChange: (value: string) => void;
  id: string;
}

function parseInOptions(rules: string): string[] | null {
  const part = rules.split('|').find((r) => r.startsWith('in:'));
  if (!part) return null;
  return part.slice(3).split(',').filter(Boolean);
}

export function EggVariableField({ variable, value, onChange, id }: Props) {
  const rules = variable.rules.split('|');
  const required = rules.includes('required');
  const options = parseInOptions(variable.rules);

  if (options) {
    return (
      <select id={id} value={value} onChange={(e) => onChange(e.target.value)} disabled={!variable.is_editable} required={required}>
        {options.map((opt) => (
          <option key={opt} value={opt}>
            {opt}
          </option>
        ))}
      </select>
    );
  }

  if (rules.includes('integer')) {
    return (
      <input
        id={id}
        type="number"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        disabled={!variable.is_editable}
        required={required}
      />
    );
  }

  return (
    <input id={id} value={value} onChange={(e) => onChange(e.target.value)} disabled={!variable.is_editable} required={required} />
  );
}

export function eggVariableHint(rules: string): string {
  return rules
    .split('|')
    .filter((r) => r && !r.startsWith('in:') && r !== 'integer' && r !== 'required')
    .join(' · ');
}
