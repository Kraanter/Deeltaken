FROM mcr.microsoft.com/dotnet/sdk:8.0
WORKDIR /source

COPY *.csproj .
RUN dotnet restore

RUN dotnet tool install --global csharp-ls

ENV PATH="${PATH}:/root/.dotnet/tools"

COPY *.sh .
RUN chmod +x script.sh
RUN chmod +x lsp.sh

# Run the container while input is supplied
ENTRYPOINT ["cat"]
