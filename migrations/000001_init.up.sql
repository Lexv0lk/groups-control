-- Схема БД сервиса управления группами людей.
-- gen_random_uuid() доступна в ядре PostgreSQL начиная с 13-й версии,
-- поэтому дополнительные расширения не требуются.

-- Функция-триггер: автоматически обновляет updated_at при UPDATE строки.
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at := now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Группы образуют дерево: parent_id ссылается на родительскую группу.
-- ON DELETE RESTRICT запрещает удаление группы, у которой есть дочерние.
CREATE TABLE groups (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    parent_id  UUID REFERENCES groups (id) ON DELETE RESTRICT,
    name       TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_groups_parent_id ON groups (parent_id);

CREATE TRIGGER trg_groups_set_updated_at
    BEFORE UPDATE ON groups
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Люди привязаны к группе (group_id обязателен).
-- ON DELETE RESTRICT запрещает удаление группы, в которой есть люди.
CREATE TABLE people (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    first_name TEXT NOT NULL CHECK (char_length(first_name) BETWEEN 1 AND 255),
    last_name  TEXT NOT NULL CHECK (char_length(last_name) BETWEEN 1 AND 255),
    birth_year INTEGER NOT NULL CHECK (birth_year BETWEEN 1900 AND 2100),
    group_id   UUID NOT NULL REFERENCES groups (id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_people_group_id ON people (group_id);

CREATE TRIGGER trg_people_set_updated_at
    BEFORE UPDATE ON people
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();
