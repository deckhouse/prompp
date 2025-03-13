import React, { FC } from 'react';
import { Alert } from 'reactstrap';
import Graph from './Graph';
import { QueryParams, ExemplarData } from '../../types/types';
import { isPresent } from '../../utils';
import { GraphDisplayMode } from './Panel';
import { useTranslation } from 'react-i18next';

interface GraphTabContentProps {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  data: any;
  exemplars: ExemplarData;
  displayMode: GraphDisplayMode;
  useLocalTime: boolean;
  showExemplars: boolean;
  handleTimeRangeSelection: (startTime: number, endTime: number) => void;
  lastQueryParams: QueryParams | null;
  id: string;
}

export const GraphTabContent: FC<GraphTabContentProps> = ({
  data,
  exemplars,
  displayMode,
  useLocalTime,
  lastQueryParams,
  showExemplars,
  handleTimeRangeSelection,
  id,
}) => {
  const { t } = useTranslation('graph');
  if (!isPresent(data)) {
    return <Alert color="light">{t('No data queried yet')}</Alert>;
  }
  if (data.result.length === 0) {
    return <Alert color="secondary">{t('Empty query result')}</Alert>;
  }
  if (data.resultType !== 'matrix') {
    return (
      <Alert color="danger">
        {t("Query result is of wrong type '{data.resultType}', should be 'matrix' (range vector).")}
      </Alert>
    );
  }
  return (
    <Graph
      data={data}
      exemplars={exemplars}
      displayMode={displayMode}
      useLocalTime={useLocalTime}
      showExemplars={showExemplars}
      handleTimeRangeSelection={handleTimeRangeSelection}
      queryParams={lastQueryParams}
      id={id}
    />
  );
};
