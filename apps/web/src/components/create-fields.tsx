import { Palette } from 'lucide-react';
import { accentColors } from '../data';

type CreateFieldsProps = {
  title: string;
  setTitle: (value: string) => void;
  description: string;
  setDescription: (value: string) => void;
  placeholder: string;
  color?: string;
  setColor?: (value: string) => void;
  showColor?: boolean;
};

export function CreateFields({
  title,
  setTitle,
  description,
  setDescription,
  placeholder,
  color,
  setColor,
  showColor = false,
}: CreateFieldsProps) {
  return (
    <div className="create-fields">
      <input
        autoFocus
        className="composer-title"
        placeholder={placeholder}
        value={title}
        onChange={(event) => setTitle(event.target.value)}
      />
      <textarea
        className="composer-description"
        placeholder="Add a description (optional)"
        value={description}
        onChange={(event) => setDescription(event.target.value)}
      />
      {showColor && color && setColor && (
        <div className="create-color-picker" role="group" aria-label="Color">
          <span>Color</span>
          <div className="accent-picker">
            {accentColors.map((accent) => (
              <button
                key={accent.value}
                type="button"
                className={color === accent.value ? 'active' : ''}
                style={{ backgroundColor: accent.value }}
                onClick={() => setColor(accent.value)}
                aria-label={accent.name}
                aria-pressed={color === accent.value}
              />
            ))}
            <label
              className={`custom-color-picker ${!accentColors.some((accent) => accent.value === color) ? 'active' : ''}`}
              style={{ backgroundColor: color }}
              aria-label="Custom color"
            >
              <Palette size={14} aria-hidden="true" />
              <input
                type="color"
                value={color}
                onChange={(event) => setColor(event.target.value)}
                aria-label="Choose a custom color"
              />
            </label>
          </div>
        </div>
      )}
    </div>
  );
}
