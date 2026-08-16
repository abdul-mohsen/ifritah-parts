import type { Specification } from '../types';

interface Props {
  specs: Specification[];
}

export function SpecificationTable({ specs }: Props) {
  if (!specs || specs.length === 0) return null;

  return (
    <div style={{ marginTop: '8px' }}>
      <div style={{ fontSize: '0.75rem', fontWeight: 600, color: '#374151', marginBottom: '4px' }}>
        Specifications
      </div>
      <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '0.8rem' }}>
        <tbody>
          {specs.map((sp, i) => (
            <tr key={i} style={{ borderBottom: '1px solid #f3f4f6' }}>
              <td style={{ padding: '3px 8px 3px 0', color: '#6b7280', whiteSpace: 'nowrap', width: '45%' }}>
                {sp.name}
              </td>
              <td style={{ padding: '3px 0', color: '#111827', fontWeight: 500 }}>
                {sp.value}{sp.unit ? ` ${sp.unit}` : ''}
                {sp.warning && (
                  <span style={{ marginLeft: '4px', color: '#f59e0b', fontSize: '0.7rem' }}>
                    ⚠ {sp.warning}
                  </span>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
