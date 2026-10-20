package control

import (
	"context"
	"errors"
	"fmt"
	"log"
)

// cycle sincroniza configuración antes de ejecutar órdenes remotas. El diario se escribe antes de ejecutar o confirmar para evitar repetir acciones tras un reinicio.
func (r *Runtime) cycle(ctx context.Context) error {
	config, err := r.client.Configuration(ctx)
	if err != nil {
		return err
	}
	applyErr := r.apply(config)
	if applyErr != nil {
		r.mu.Lock()
		if r.configError == nil || r.configError.Revision != config.Revision || r.configError.Message != applyErr.Error() {
			log.Printf("Configuracion de audio rechazada (revision %d): %v", config.Revision, applyErr)
		}
		r.configError = &ConfigError{config.Revision, "unsupported_configuration", applyErr.Error()}
		r.mu.Unlock()
	}
	if r.audio != nil {
		if err := r.finishPending(ctx, applyErr == nil); err != nil {
			return err
		}
	}
	commands, err := r.client.Commands(ctx)
	if err != nil {
		return err
	}
	var previous int64
	for _, cmd := range commands {
		if cmd.ID == "" || cmd.Sequence <= previous {
			return errors.New("lote de órdenes inválido")
		}
		previous = cmd.Sequence
		result, seen := r.journal.results[cmd.ID]
		if !seen {
			if r.audio != nil {
				if err := r.supersede(cmd); err != nil {
					return err
				}
			}
			if err := r.journal.Record(cmd.ID, nil); err != nil {
				return fmt.Errorf("persistir inicio: %w", err)
			}
			executed := r.execute(cmd)
			if executed.Outcome == "pending" {
				continue
			}
			result = &executed
			if err := r.journal.Record(cmd.ID, result); err != nil {
				return fmt.Errorf("persistir resultado: %w", err)
			}
		} else if result == nil {
			r.mu.Lock()
			pending := r.pending[cmd.ID] != nil
			r.mu.Unlock()
			if pending {
				continue
			}
			unknown := failed("execution_unknown", "El proceso terminó sin persistir el resultado; no se repite la acción")
			result = &unknown
			if err := r.journal.Record(cmd.ID, result); err != nil {
				return err
			}
		}
		if err := r.client.Confirm(ctx, cmd.ID, *result); err != nil {
			return err
		}
	}
	return nil
}
