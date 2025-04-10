import React, { FC, useEffect, useState } from 'react';
import { Form, Button, ButtonGroup } from 'reactstrap';
import { useTranslation } from 'react-i18next';
import { DEFAULT_LANGUAGE, LOCAL_STORAGE_LANGUAGE_KEY } from './i18n';

export const LanguageToggle: FC = () => {
  const { i18n } = useTranslation();

  const [value, setValue] = useState(() => localStorage.getItem(LOCAL_STORAGE_LANGUAGE_KEY) || DEFAULT_LANGUAGE);

  useEffect(() => {
    localStorage.setItem(LOCAL_STORAGE_LANGUAGE_KEY, value);
  }, [value]);

  const changeLanguage = (language: string) => {
    void i18n.changeLanguage(language);

    setValue(language);
  };

  return (
    <Form className="ml-auto mr-3" inline>
      <ButtonGroup size="sm">
        <Button color="secondary" active={value === 'en'} onClick={() => changeLanguage('en')}>
          EN
        </Button>
        <Button color="secondary" active={value === 'ru'} onClick={() => changeLanguage('ru')}>
          RU
        </Button>
      </ButtonGroup>
    </Form>
  );
};
