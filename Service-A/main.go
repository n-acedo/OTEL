package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/zipkin"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

type ResponseWeather struct {
	Celsius    float64 `json:"temp_C"`
	Fahrenheit float64 `json:"temp_F"`
	Kelvin     float64 `json:"temp_K"`
}

func Tracing() {
	exporter, err := zipkin.New("http://zipkin:9411/api/v2/spans")

	if err != nil {
		panic("Error to create exporter")
	}

	traceProvider := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("service-a"),
		)),
	)

	otel.SetTracerProvider(traceProvider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
}

func main() {
	Tracing()

	http.HandleFunc("/", Handler)
	http.ListenAndServe(":8082", nil)
}

func Handler(w http.ResponseWriter, r *http.Request) {
	cepInput := struct {
		Cep string `json:"cep"`
	}{}

	err := json.NewDecoder(r.Body).Decode(&cepInput)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if !isValidCEP(cepInput.Cep) {
		http.Error(w, "invalid zipcode", http.StatusUnprocessableEntity)
		return
	}

	ctx, span := otel.Tracer("service-a").Start(r.Context(), "start")
	defer span.End()

	response, err := requestServiceB(ctx, cepInput.Cep)
	if err != nil {
		if err.Error() == "404 Not Found" {
			http.Error(w, "cannot find zipcode", http.StatusNotFound)
			return
		}

		http.Error(w, "external error - service B", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func isValidCEP(cep string) bool {
	match, _ := regexp.MatchString(`^\d{8}$`, cep)
	return match
}

func requestServiceB(ctx context.Context, cep string) (*ResponseWeather, error) {
	_, span := otel.Tracer("service-a").Start(ctx, "Send request to Service B")
	span.End()

	url := fmt.Sprintf("http://goapp-b:8083?cep=%s", cep)

	res, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, fmt.Errorf(res.Status)
	}

	var result *ResponseWeather
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result, nil
}
