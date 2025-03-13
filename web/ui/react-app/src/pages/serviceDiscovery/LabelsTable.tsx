import React, { FC, useState } from 'react';
import { Badge, Table } from 'reactstrap';
import { TargetLabels } from './Services';
import { ToggleMoreLess } from '../../components/ToggleMoreLess';
import CustomInfiniteScroll, { InfiniteScrollItemsProps } from '../../components/CustomInfiniteScroll';
import { useTranslation } from 'react-i18next';

interface LabelProps {
  value: TargetLabels[];
  name: string;
}

const formatLabels = (labels: Record<string, string> | string) => {
  return Object.entries(labels).map(([key, value]) => {
    return (
      <div key={key}>
        <Badge color="primary" className="mr-1">
          {`${key}="${value}"`}
        </Badge>
      </div>
    );
  });
};

const LabelsTableContent: FC<InfiniteScrollItemsProps<TargetLabels>> = ({ items }) => {
  const { t } = useTranslation();
  return (
    <Table size="sm" bordered hover striped>
      <thead>
        <tr>
          <th>{t('Discovered Labels')}</th>
          <th>{t('Target Labels')}</th>
        </tr>
      </thead>
      <tbody>
        {items.map((_, i) => {
          return (
            <tr key={i}>
              <td>{formatLabels(items[i].discoveredLabels)}</td>
              {items[i].isDropped ? (
                <td style={{ fontWeight: 'bold' }}>{t('Dropped')}</td>
              ) : (
                <td>{formatLabels(items[i].labels)}</td>
              )}
            </tr>
          );
        })}
      </tbody>
    </Table>
  );
};

export const LabelsTable: FC<LabelProps> = ({ value, name }) => {
  const [showMore, setShowMore] = useState(false);

  return (
    <>
      <div>
        <ToggleMoreLess
          event={(): void => {
            setShowMore(!showMore);
          }}
          showMore={showMore}
        >
          <span className="target-head">{name}</span>
        </ToggleMoreLess>
      </div>
      {showMore ? <CustomInfiniteScroll allItems={value} child={LabelsTableContent} /> : null}
    </>
  );
};
