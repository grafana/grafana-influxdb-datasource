import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { PartListSection, type PartParams } from './PartListSection';

const parts: Array<{ name: string; params: PartParams }> = [
  {
    name: 'mean',
    params: [{ value: 'temperature', options: () => Promise.resolve(['temperature', 'humidity']) }],
  },
  {
    name: 'alias',
    params: [{ value: 'series', options: null }],
  },
];

const getNewPartOptions = () => Promise.resolve([{ label: 'max', value: 'max' }]);

describe('PartListSection', () => {
  it('renders part names as labels with specific remove actions', () => {
    render(
      <PartListSection
        parts={parts}
        getNewPartOptions={getNewPartOptions}
        onChange={jest.fn()}
        onRemovePart={jest.fn()}
        onAddNewPart={jest.fn()}
      />
    );

    expect(screen.getByText('mean', { selector: 'span' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'mean' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Remove mean' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Remove alias' })).toBeInTheDocument();
  });

  it('removes the selected part', async () => {
    const user = userEvent.setup();
    const onRemovePart = jest.fn();

    render(
      <PartListSection
        parts={parts}
        getNewPartOptions={getNewPartOptions}
        onChange={jest.fn()}
        onRemovePart={onRemovePart}
        onAddNewPart={jest.fn()}
      />
    );

    await user.click(screen.getByRole('button', { name: 'Remove alias' }));

    expect(onRemovePart).toHaveBeenCalledWith(1);
  });

  it('keeps function names next to parentheses and spaces remove buttons from parts', () => {
    render(
      <PartListSection
        parts={parts}
        getNewPartOptions={getNewPartOptions}
        onChange={jest.fn()}
        onRemovePart={jest.fn()}
        onAddNewPart={jest.fn()}
      />
    );

    const partName = screen.getByText('mean', { selector: 'span' });
    expect(getComputedStyle(partName).marginRight).toBe('0px');
    expect(partName.nextSibling?.textContent).toBe('(');
    expect(screen.getByRole('button', { name: 'Remove mean' })).toHaveStyle({ marginLeft: '4px' });
  });

  it('updates an editable parameter', async () => {
    const user = userEvent.setup();
    const onChange = jest.fn();

    render(
      <PartListSection
        parts={parts}
        getNewPartOptions={getNewPartOptions}
        onChange={onChange}
        onRemovePart={jest.fn()}
        onAddNewPart={jest.fn()}
      />
    );

    await user.click(screen.getByRole('button', { name: 'series' }));
    const input = screen.getByRole('textbox');
    await user.clear(input);
    await user.type(input, 'renamed{Enter}');

    expect(onChange).toHaveBeenCalledWith(1, ['renamed']);
  });
});
