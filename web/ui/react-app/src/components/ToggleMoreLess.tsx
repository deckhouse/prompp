import React, { FC } from 'react';
import { Button } from 'reactstrap';
import { useTranslation } from 'react-i18next';

interface ToggleMoreLessProps {
  event(): void;
  showMore: boolean;
}

export const ToggleMoreLess: FC<ToggleMoreLessProps> = ({ children, event, showMore }) => {
  const { t } = useTranslation();
  return (
    <h3>
      {children}
      <Button
        size="xs"
        onClick={event}
        style={{
          padding: '0.3em 0.3em 0.25em 0.3em',
          fontSize: '0.375em',
          marginLeft: '1em',
          verticalAlign: 'baseline',
        }}
        color="primary"
      >
        {t('show')} {showMore ? t('less') : t('more')}
      </Button>
    </h3>
  );
};
